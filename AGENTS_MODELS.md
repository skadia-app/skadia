# Agent Model Tiers

Vendor-neutral tiers used in `AGENTS.md`. Pick the lowest tier that
solves the task. Mechanical work (identical pattern across files, simple
regex rewrite) → use *No-Agent* (`sed`/`awk`/`jq`); spawning an agent
for a 30-file rename wastes tokens and adds variance.

| Tier          | Use when                                                                                                              | Anthropic   | OpenAI            | Google                |
|---------------|-----------------------------------------------------------------------------------------------------------------------|-------------|-------------------|-----------------------|
| **Reasoning** | Adversarial critique, architecture review, multi-file refactor planning, deep cross-file synthesis.                   | Opus 4.7    | GPT-5.5 Thinking  | Gemini 3.1 Pro        |
| **Standard**  | Default for implementation, codebase exploration, single-feature work, focused refactors.                             | Sonnet 4.6  | GPT-5.5           | Gemini 3 Flash        |
| **Bulk**      | Repetitive low-judgment edits where an agent is still needed (uniform fix across files with minor variation).         | Haiku 4.5   | GPT-5 mini        | Gemini 3.1 Flash-Lite |
| *No-Agent*    | Identical pattern in 20+ files, mechanical renames, JSON/YAML field updates. Use shell tools.                         | —           | —                 | —                     |

> Model names reflect tiers as of 2026-04-25. **Tier semantics** (Reasoning / Standard / Bulk) are stable; product names are not — update when vendors retire or rename a model.

## Picking a tier

1. Mechanical and identical across files? → No-Agent.
2. Judgment within a single concern? → Standard.
3. Finding contradictions, balancing tradeoffs, reasoning across unrelated files? → Reasoning.
4. Copy-paste fix that's still per-file unique? → Bulk.

## Hard limits

- Max **30 files per agent** — beyond that, split or move to No-Agent.
- One agent per file at a time. Never two parallel agents touching the same file.
- Background by default for implementation agents (`run_in_background: true` in Claude Code, equivalent elsewhere). Foreground only when the next step depends on the agent's output.

## User overrides

User can request a higher tier explicitly ("use Opus", "use Reasoning"). Honor it — they have context the heuristic doesn't.
