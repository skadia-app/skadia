# services/ml — Agent Instructions

See [`../../AGENTS.md`](../../AGENTS.md) for project-wide rules. Python ML/agent service specifics below.

## Stack + entry

- Python (FastAPI + uvicorn) on port 8181, NATS consumer + REST endpoints.
- Async-first: `asyncpg` for Postgres, async NATS subscriptions, FastAPI async handlers.
- Entry point: `main.py` (lifespan-managed NATS consumer + DB pool).
- Container build is independent from `services/api/`; shares the `skadia-ml-1` runtime.

## Layout

- `agent/` — SQL agent (LLM-driven query generation), templates, tool schemas.
- `chat/` — assembler, profile router, skill loader, skill definitions.
- `analytics/` — feature store, clustering, changepoints, DFA-α1, HRV, LLM insights.
- `enrichment/` — activity-track enrichment pipelines.
- `ocr/` — invoice OCR via transformers + Pillow.
- `models/` — model loading + caching.
- `api/` — REST endpoints.
- `tests/` — pytest; `db.py`, `nats_consumer.py`, `progress.py`, `config.py` at root.

## Conventions

- **Async by default.** Blocking I/O (sync DB driver, `requests`, file reads in handlers) is a bug. Use `asyncpg`, `httpx`, `aiofiles`, or wrap sync libs with `asyncio.to_thread`.
- **Type hints everywhere.** Match the existing style — `pool: asyncpg.Pool | None`, not `Optional[asyncpg.Pool]`.
- **Pydantic for FastAPI request/response models** — never raw dicts on public endpoints.
- **No CUDA assumptions.** Targets AVX2-less CPU + AMD GPU (Vulkan/ROCm). Torch and transformers code paths must work on CPU; ROCm-specific code goes behind a runtime check.
- **Logging via `pythonjsonlogger`** (configured in `main.py`) — use `logger.info(...)` with structured `extra={...}`, not f-strings into `print`.
- **Config via env vars** read in `config.py`; never hard-code endpoints, model names, or credentials.

## Database

- Connect via the async pool from `db.py` (`get_pool()`); never open ad-hoc connections.
- Same Postgres instance + schema as `services/api/`. The Go service owns migrations — do not write SQL DDL here.
- Respect RLS (`SET LOCAL app.current_user_id`) for user-scoped queries; service-tier queries that intentionally bypass RLS must be commented as such.
- Shared schema rules live in [`../api/AGENTS.md`](../api/AGENTS.md)
  (column drops, session variables, GDPR) and [`../../AGENTS.md`](../../AGENTS.md)
  (no precomputed enrichment columns).

## NATS

- Consumer registration in `nats_consumer.py`. New subjects must be added there + the corresponding handler module.
- Treat every consumer as idempotent — messages may be redelivered (JetStream retry).
- Long-running work goes into `asyncio.create_task(...)`; never block the consumer callback.

## LLM / agent code

- The SQL agent (`agent/sql_agent.py`) executes model-generated SQL — every code path that builds a query for the agent must validate or whitelist via `sqlglot` first; never concatenate user input into SQL.
- LLM API calls use the `openai` client (compatible with local endpoints). Prompts that ship to production live under `chat/skills/` (versioned, not inlined).
- Chat profile / skill definitions are YAML-loaded at startup via `skill_loader.py` — schema changes go through there.

## Tests

- `pytest` from `services/ml/`. Tests rely on the shared Postgres (same `TEST_DATABASE_URL` convention as the Go side).
- Async tests use `pytest-asyncio`; mark with `@pytest.mark.asyncio`.
- Don't mock `asyncpg` — use the real pool against a test DB so RLS, JSONB encoding, and timezone behavior stay honest.
