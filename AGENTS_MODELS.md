# Agent Model Tiers

Vendor-neutral tiers used in `AGENTS.md`. Pick the lowest tier that
solves the task. Mechanical work (identical pattern across files, simple
regex rewrite) → use *No-Agent* (`sed`/`awk`/`jq`); spawning an agent
for a 30-file rename wastes tokens and adds variance.

| Tier          | Use when                                                                                                      | Selection rule                                                                                                      |
|---------------|---------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| **Reasoning** | Adversarial critique, architecture review, multi-file refactor planning, deep cross-file synthesis.           | Use the strongest available reasoning-capable model or the highest reasoning-effort setting exposed by the harness.  |
| **Standard**  | Default for implementation, codebase exploration, single-feature work, focused refactors.                     | Use the harness default strong coding model.                                                                         |
| **Bulk**      | Repetitive low-judgment edits where an agent is still needed (uniform fix across files with minor variation). | Use a cheaper/faster model only if it preserves correctness for the specific edit.                                   |
| *No-Agent*    | Identical pattern in 20+ files, mechanical renames, JSON/YAML field updates. Use shell tools.                 | Do not spawn an agent.                                                                                               |

These tiers describe capability and cost, not a one-to-one product
mapping. Some harnesses expose separate model names; others expose
reasoning-effort settings on the same model. Use the closest available
control that matches the tier semantics.

## Picking a tier

1. Mechanical and identical across files? → No-Agent.
2. Judgment within a single concern? → Standard.
3. Finding contradictions, balancing tradeoffs, reasoning across unrelated files? → Reasoning.
4. Copy-paste fix that's still per-file unique? → Bulk.

## Hard limits

- Max **30 files per agent** — beyond that, split or move to No-Agent.
- One agent per file at a time. Never two parallel agents touching the same file.
- Background by default for implementation agents where the harness supports it. Foreground only when the next step depends on the agent's output.

## User overrides

User can request a higher tier explicitly ("use Reasoning", "use the
strongest model"). Honor it — they have context the heuristic doesn't.
