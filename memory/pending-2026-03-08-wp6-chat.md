## docs/08_API_REFERENCE.md — Chat API Endpoints
**Suche nach:** chat
**Delta:** Neue Chat-Endpoints unter `/api/v1/chat/`:
- `GET /chat/sessions` — Liste aller Sessions (50 neueste)
- `POST /chat/sessions` — Neue Session erstellen
- `GET /chat/sessions/{id}/messages` — Nachrichten einer Session (bis 200)
- `DELETE /chat/sessions/{id}` — Session löschen
- `POST /chat/sessions/{id}/send` — Nachricht senden (SSE-Stream zurück)
- Rate Limit: 10 Nachrichten/Minute pro User (in-memory)

## docs/04_ML_AI_SERVICE.md — Chat/Agent
**Suche nach:** chat
**Delta:** ML-Service (`services/ml/`) erhält neues Chat-Modul:
- `services/ml/agent/templates.py` — Template-Queries ohne LLM (4 Templates: last_activities, weekly_summary, hrv_trend, readiness)
- `services/ml/agent/sql_agent.py` — Read-only SQL-Validator + Executor (TABLE_WHITELIST, SCHEMA_CONTEXT)
- `services/ml/agent/tools.py` — 6 LLM Tool-Definitionen + Executor (query_activities, get_hrv_trend, get_training_load, compare_periods, get_readiness, get_gear_stats)
- `services/ml/api/chat.py` — FastAPI POST `/ml/chat` (SSE-Stream, 3-Layer Fallback: Template → LLM Tool-Use → Error)
- Router in `main.py` registriert: `app.include_router(chat_router)`

## docs/07_DATABASE_SCHEMA.md — Chat-Tabellen
**Suche nach:** chat_sessions
**Delta:** Migrations 000106/000107 (geplant) erstellen:
- `chat_sessions (id UUID PK, user_id UUID FK, title TEXT, created_at, updated_at)`
- `chat_messages (id UUID PK, session_id UUID FK, role TEXT, content TEXT, tool_calls JSONB, sources JSONB, created_at)`

## CHANGELOG.md
**Delta:**
### Added
- **WP6 Chat/Agent Backend**: Go REST + SSE chat handler with rate limiting (10/min), ML streaming endpoint with 3-layer fallback (templates → LLM tool-use → error), 6 agent tools (activities, HRV, training load, readiness, gear, period comparison)
