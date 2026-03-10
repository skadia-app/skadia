"""Chat endpoint with 3-layer fallback: Templates → Tool-Use LLM → Error.

Streams responses via SSE for real-time frontend updates.

Pipeline (WP8 + WP3 Multi-Hop):
  1. Load ModelProfile from user settings (or CHAT_PROFILE_DEFAULT)
  2. Route query to domains via router.route_query()
  3. Load skill modules, assemble system prompt via assembler
  4. Filter tools to active domains via get_tools_for_domains()
  5. ReAct loop (up to MAX_HOPS=5):
       a. LLM call with profile-aware sampling parameters
       b. If finish_reason == "tool_calls": execute tools, append results, continue
       c. If finish_reason == "stop": stream final answer, break
       d. Last hop: tools disabled to force text answer
       e. Context compression from hop 3 onwards
"""
import json
import logging
from typing import AsyncGenerator
from uuid import UUID

from fastapi import APIRouter
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from agent.templates import match_template
from agent.tools import execute_tool, get_tool_definitions
from config import (
    CHAT_PROFILE_DEFAULT,
    LLM_API_KEY,
    LLM_BASE_URL,
    get_llm_client,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/ml", tags=["chat"])

LLM_MODEL = __import__("os").environ.get("LLM_MODEL", "skadia-default")


class ChatRequest(BaseModel):
    user_id: UUID
    session_id: UUID
    message_id: str
    message: str
    history: list[dict] = []


@router.post("/chat")
async def chat(req: ChatRequest):
    """Stream chat response via SSE."""
    return StreamingResponse(
        _chat_stream(req),
        media_type="text/event-stream",
    )


# ---------------------------------------------------------------------------
# Profile loading
# ---------------------------------------------------------------------------

async def _load_user_profile(pool, user_id):
    """Load the user's chat profile from ml_user_settings, falling back to
    CHAT_PROFILE_DEFAULT if no setting is present or an error occurs.

    The DB row stores a JSON object with two optional keys:
      - "base_profile": one of "tiny"/"small"/"medium"/"large"/"auto"
      - "overrides": dict of ModelProfile field overrides
    """
    from chat.profiles import get_profile
    try:
        async with pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT value_json FROM ml_user_settings "
                "WHERE user_id = $1 AND setting_key = 'chat_profile'",
                user_id,
            )
        if row:
            data = row["value_json"] or {}
            base = data.get("base_profile", CHAT_PROFILE_DEFAULT)
            overrides = data.get("overrides")
            return get_profile(base, model_name=LLM_MODEL, overrides=overrides)
    except Exception as exc:  # noqa: BLE001
        logger.debug("_load_user_profile failed (using default): %s", exc)
    return get_profile(CHAT_PROFILE_DEFAULT, model_name=LLM_MODEL)


# ---------------------------------------------------------------------------
# Main streaming handler
# ---------------------------------------------------------------------------

async def _chat_stream(req: ChatRequest) -> AsyncGenerator[str, None]:
    """Main chat logic with routing, skills, profile-aware tool-use."""
    from db import get_pool
    pool = await get_pool()
    user_id = req.user_id

    # ── 1. Load profile ────────────────────────────────────────────────────
    try:
        from chat.profiles import get_profile
        profile = await _load_user_profile(pool, user_id)
    except Exception as exc:  # noqa: BLE001
        logger.warning("Profile loading failed, using hardcoded defaults: %s", exc)
        from chat.profiles import ModelProfile
        profile = ModelProfile()

    # ── 2. Route query → domains ───────────────────────────────────────────
    try:
        from chat.router import route_query, get_tools_for_domains
        route = route_query(req.message, profile)
        domains = route.domains
    except Exception as exc:  # noqa: BLE001
        logger.warning("Router failed, using default domains: %s", exc)
        domains = ["activities", "health"]

    # ── 3. Load skills + assemble system prompt ────────────────────────────
    try:
        from chat.skill_loader import load_skills
        from chat.assembler import assemble_system_prompt
        all_skills = load_skills()
        active_skills = [all_skills[d] for d in domains if d in all_skills]
        system_prompt = assemble_system_prompt(profile, active_skills)
    except Exception as exc:  # noqa: BLE001
        logger.warning("Skill/assembler failed, using fallback prompt: %s", exc)
        from datetime import date as _date
        system_prompt = (
            f"Du bist ein Trainingsanalyst. Heute ist {_date.today().isoformat()}. "
            "Antworte auf Deutsch, knapp und präzise."
        )

    # ── 4. Filter tools ────────────────────────────────────────────────────
    try:
        from chat.router import get_tools_for_domains as _gtfd
        tool_names = _gtfd(domains, profile)
        tool_definitions = get_tool_definitions(tool_names) if tool_names else get_tool_definitions()
    except Exception as exc:  # noqa: BLE001
        logger.warning("Tool filtering failed, using all tools: %s", exc)
        tool_definitions = get_tool_definitions()

    # ── Layer A: Template fast-path (no LLM needed) ────────────────────────
    template_match = match_template(req.message)
    if template_match:
        yield _sse("chat.thinking", req.message_id, step="Template-Antwort...")
        tmpl = template_match["template"]
        try:
            rows = await pool.fetch(tmpl["sql"], user_id, *tmpl["params"](template_match["match"]))
            result_text = tmpl["format"] + "\n" + _format_rows(rows)
            yield _sse("chat.chunk", req.message_id, delta=result_text)
            yield _sse("chat.done", req.message_id, sources=["template"])
            return
        except Exception as e:
            logger.warning("Template query failed: %s", e)
            # Fall through to LLM

    # ── Layer B: LLM with multi-hop ReAct loop ────────────────────────────
    MAX_HOPS = 5
    try:
        llm = get_llm_client()
        if llm is None:
            from openai import AsyncOpenAI
            llm = AsyncOpenAI(base_url=LLM_BASE_URL, api_key=LLM_API_KEY)

        yield _sse("chat.thinking", req.message_id, step="Analysiere Frage...")

        # Build message list respecting profile.history_messages limit
        messages = [{"role": "system", "content": system_prompt}]
        for msg in (req.history or [])[-profile.history_messages:]:
            if msg.get("role") in ("user", "assistant") and msg.get("content"):
                messages.append({"role": msg["role"], "content": msg["content"]})
        messages.append({"role": "user", "content": req.message})

        # Respect profile.tool_result_limit — cap how many chars each
        # result may contribute so context doesn't blow up on small models.
        result_char_limit = profile.tool_result_limit * 200

        hop = 0
        while hop < MAX_HOPS:
            # Last hop: disable tools to force a plain-text answer
            tools_arg = tool_definitions if (tool_definitions and hop < MAX_HOPS - 1) else None

            response = await llm.chat.completions.create(
                model=LLM_MODEL,
                messages=messages,
                tools=tools_arg,
                stream=True,
                temperature=profile.temperature,
                top_p=profile.top_p,
                presence_penalty=profile.presence_penalty,
                max_tokens=profile.max_response_tokens,
            )

            # Accumulate tool-call deltas keyed by tc.index.
            # Streaming splits arguments across many chunks — we must join them
            # before attempting json.loads().
            tool_call_accum: dict[int, dict] = {}
            tool_call_ids: dict[int, str] = {}
            finish_reason: str | None = None

            async for chunk in response:
                if not chunk.choices:
                    continue
                choice = chunk.choices[0]
                delta = choice.delta

                if delta.tool_calls:
                    for tc in delta.tool_calls:
                        idx = tc.index
                        if idx not in tool_call_accum:
                            tool_call_accum[idx] = {"name": "", "arguments": ""}
                        if tc.id:
                            tool_call_ids[idx] = tc.id
                        if tc.function:
                            if tc.function.name:
                                tool_call_accum[idx]["name"] = tc.function.name
                            if tc.function.arguments:
                                tool_call_accum[idx]["arguments"] += tc.function.arguments

                if delta.content:
                    yield _sse("chat.chunk", req.message_id, delta=delta.content)

                if choice.finish_reason:
                    finish_reason = choice.finish_reason

            # ── Post-stream: decide what to do next ──────────────────────────
            if finish_reason == "tool_calls" and tool_call_accum:
                # Execute all accumulated tool calls
                assistant_tool_calls = []
                tool_results = []

                for idx, entry in sorted(tool_call_accum.items()):
                    tool_name = entry["name"]
                    raw_args = entry["arguments"]
                    call_id = tool_call_ids.get(idx, f"call_{idx}")

                    yield _sse("chat.tool_call", req.message_id,
                               tool=tool_name, args=raw_args)

                    assistant_tool_calls.append({
                        "id": call_id,
                        "type": "function",
                        "function": {"name": tool_name, "arguments": raw_args},
                    })

                    try:
                        args = json.loads(raw_args) if raw_args else {}
                        result = await execute_tool(pool, user_id, tool_name, args)
                        result_str = json.dumps(result, default=str)
                        yield _sse("chat.tool_result", req.message_id,
                                   tool=tool_name,
                                   summary=result_str[:500])
                    except Exception as e:
                        result_str = f"Error: {e}"
                        yield _sse("chat.tool_result", req.message_id,
                                   tool=tool_name, summary=result_str)

                    tool_results.append({
                        "role": "tool",
                        "tool_call_id": call_id,
                        "content": result_str[:result_char_limit],
                    })

                # Append assistant tool-call message + tool results to context
                messages.append({
                    "role": "assistant",
                    "content": None,
                    "tool_calls": assistant_tool_calls,
                })
                messages.extend(tool_results)

                hop += 1

                # Context compression starting from hop 3
                if hop >= 3:
                    _compress_old_tool_results(messages, hop)

                yield _sse("chat.thinking", req.message_id,
                           step=f"Schritt {hop}/{MAX_HOPS}...")
                continue  # next hop

            elif finish_reason == "stop":
                break  # Final answer already streamed

            else:
                # "length" or unknown — stop without further hops
                break

        # MAX_HOPS exhausted without a stop — force a final summary
        if hop >= MAX_HOPS:
            messages.append({
                "role": "user",
                "content": "Fasse die bisher gesammelten Daten zusammen.",
            })
            final_response = await llm.chat.completions.create(
                model=LLM_MODEL,
                messages=messages,
                stream=True,
                temperature=profile.temperature,
                top_p=profile.top_p,
                presence_penalty=profile.presence_penalty,
                max_tokens=profile.max_response_tokens,
            )
            async for chunk in final_response:
                if not chunk.choices:
                    continue
                delta = chunk.choices[0].delta
                if delta.content:
                    yield _sse("chat.chunk", req.message_id, delta=delta.content)

        # chat.done is always emitted here — both the normal path (finish_reason="stop")
        # and the forced-summary path (hop >= MAX_HOPS) fall through to this line.
        yield _sse("chat.done", req.message_id, sources=["llm"])

    except Exception as e:
        logger.warning("LLM chat failed: %s", e)
        yield _sse("chat.error", req.message_id, error=f"Chat nicht verfügbar: {e}")


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _compress_old_tool_results(messages: list, current_hop: int) -> None:
    """Truncate tool-result messages from hops that are >= 2 hops old.

    Walks the message list to find assistant messages that carry tool_calls
    (each such message marks the boundary of one hop).  Everything before the
    second-to-last such boundary is considered "old" and any tool-role message
    there is truncated to 400 characters to prevent context blow-up.

    Args:
        messages:    The live message list (mutated in-place).
        current_hop: The hop counter *after* the latest tool execution.
    """
    # Find indices of assistant messages that triggered tool calls (hop boundaries)
    hop_boundaries: list[int] = [
        i for i, msg in enumerate(messages)
        if msg.get("role") == "assistant" and msg.get("tool_calls")
    ]

    # Need at least 3 boundaries before compression makes sense
    if len(hop_boundaries) < 3:
        return

    # Compress everything before the second-to-last boundary
    cutoff = hop_boundaries[-2]
    for i in range(cutoff):
        if messages[i].get("role") == "tool":
            content = messages[i].get("content", "")
            if isinstance(content, str) and len(content) > 400:
                messages[i]["content"] = content[:400] + "...[komprimiert]"


def _sse(event_type: str, msg_id: str, **kwargs) -> str:
    data = {"type": event_type, "id": msg_id, **kwargs}
    return f"data: {json.dumps(data, default=str)}\n\n"


def _format_rows(rows: list) -> str:
    if not rows:
        return "Keine Daten gefunden."
    lines = []
    for r in rows[:20]:
        d = dict(r)
        parts = [f"{k}: {v}" for k, v in d.items() if k != "id"]
        lines.append(" | ".join(parts))
    return "\n".join(lines)
