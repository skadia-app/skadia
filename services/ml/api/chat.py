"""Chat endpoint with 3-layer fallback: Templates → Tool-Use LLM → Error.

Streams responses via SSE for real-time frontend updates.
"""
import json
import logging
import os
from typing import AsyncGenerator
from uuid import UUID

from fastapi import APIRouter
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from agent.templates import match_template
from agent.tools import TOOL_DEFINITIONS, execute_tool

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/ml", tags=["chat"])

LLM_BASE_URL = os.environ.get("LLM_BASE_URL", "http://litellm:4000/v1")
LLM_API_KEY = os.environ.get("LLM_API_KEY", "none")
LLM_MODEL = os.environ.get("LLM_MODEL", "skadia-default")

SYSTEM_PROMPT = """Du bist ein Trainingsanalyst für einen erfahrenen Sportler. Heute ist {today}.

Regeln:
- Sprich auf Augenhöhe, knapp und direkt — wie ein kompetenter Trainingspartner.
- Zahlen > Prosa. Konkrete Daten, Distanzen (km), Zeiten, Pace (min/km), HR nennen.
- Wenn du Tool-Ergebnisse bekommst: fasse sie natürlichsprachlich zusammen. Nenne konkrete Werte aus den Daten (Datum, Titel, Distanz, Ort etc.). Gib dem User nie rohe JSON-Daten.
- Wenn keine Daten gefunden werden, sag das klar.
- Antworte auf Deutsch.

Sport-Typ Mapping (User sagt → DB-Wert):
Laufen/Joggen→running, Radfahren/Rad→cycling, Wandern→hiking, Gehen/Spazieren→walking,
Schwimmen→swimming, Skifahren→alpine_skiing, Langlauf→cross_country_skiing,
Krafttraining→strength_training, Trail→trail_running, MTB/Mountainbike→mountain_biking
"""


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


async def _chat_stream(req: ChatRequest) -> AsyncGenerator[str, None]:
    """Main chat logic with fallback layers."""
    from db import get_pool
    pool = await get_pool()
    user_id = req.user_id

    # Layer A: Check templates first (works without LLM)
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

    # Layer B: Try LLM with tool-use
    try:
        from openai import AsyncOpenAI

        llm = AsyncOpenAI(base_url=LLM_BASE_URL, api_key=LLM_API_KEY)

        yield _sse("chat.thinking", req.message_id, step="Analysiere Frage...")

        from datetime import date as datemod
        system = SYSTEM_PROMPT.format(today=datemod.today().isoformat())
        messages = [{"role": "system", "content": system}]
        for msg in req.history[-10:]:
            if msg.get("role") in ("user", "assistant") and msg.get("content"):
                messages.append({"role": msg["role"], "content": msg["content"]})
        messages.append({"role": "user", "content": req.message})

        response = await llm.chat.completions.create(
            model=LLM_MODEL,
            messages=messages,
            tools=TOOL_DEFINITIONS,
            stream=True,
        )

        # Accumulate tool-call deltas keyed by tc.index.
        # Streaming splits arguments across many chunks — we must join them
        # before attempting json.loads().
        tool_call_accum: dict[int, dict] = {}
        # Track tool call IDs for the follow-up message
        tool_call_ids: dict[int, str] = {}

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

            # Execute accumulated tool calls once the LLM signals it is done.
            if choice.finish_reason == "tool_calls" and tool_call_accum:
                # Build assistant message with tool_calls for the follow-up
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
                        "content": result_str[:2000],
                    })

                tool_call_accum.clear()

                # Second LLM call: send tool results back so it can
                # formulate a natural-language answer.
                messages.append({
                    "role": "assistant",
                    "content": None,
                    "tool_calls": assistant_tool_calls,
                })
                messages.extend(tool_results)

                yield _sse("chat.thinking", req.message_id,
                           step="Formuliere Antwort...")

                followup = await llm.chat.completions.create(
                    model=LLM_MODEL,
                    messages=messages,
                    stream=True,
                )

                async for chunk2 in followup:
                    if not chunk2.choices:
                        continue
                    delta2 = chunk2.choices[0].delta
                    if delta2.content:
                        yield _sse("chat.chunk", req.message_id,
                                   delta=delta2.content)

                break  # Done after follow-up

        yield _sse("chat.done", req.message_id, sources=["llm"])

    except Exception as e:
        logger.warning("LLM chat failed: %s", e)
        yield _sse("chat.error", req.message_id, error=f"Chat nicht verfügbar: {e}")


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
