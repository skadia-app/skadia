# services/api/internal/migrate — Agent Instructions

See [`../../../../AGENTS.md`](../../../../AGENTS.md) (project-wide), [`../../AGENTS.md`](../../AGENTS.md) (backend), and [`README.md`](README.md) (operational runbook: rollback ordering, Fusion-refactor 000302–308 strict order, 000308 failure recovery — read it before touching anything in that range).

## The only migrations directory

`migrations/` here is the single source of truth — no other migrations dir exists. Don't look for or create one elsewhere.

## Naming + structure

`NNNNNN_short_snake_case_description.{up,down}.sql`

- 6-digit zero-padded number, monotonically increasing.
- MUST ship both `.up.sql` and `.down.sql`. The `.down.sql` must actually undo the `.up.sql` (verify locally where feasible).
- Optional matching test inside `migrations/`: `migration_NNNNNN_test.go` (see `migrations/migration_000326_test.go`).

## Picking the next number

**Always `ls migrations/` before assigning.** Other sessions or branches may have added migrations in between — `git fetch --all` first when collaborating across branches. Take the highest existing number + 1. Never trust a remembered "next free number".

## Session-variable namespaces

When introducing a Postgres `SET LOCAL` / `current_setting` key:

- RLS context → `app.*` (e.g. `app.current_user_id`).
- Anything else (feature flags, trigger-skip gates) → `skadia.*` (e.g. `skadia.skip_rgm_cleanup`).

## GDPR

Migration adds a table with a user_id reference (direct or via FK chain) → same commit must update the GDPR registry. See **GDPR for new tables** in [`../../../../AGENTS.md`](../../../../AGENTS.md).

## Column drops

Before any `DROP COLUMN`: `grep -r "<column>"` across the repo (Go, SQL, TS). Hits outside this migration mean callers exist — coordinate with the user.

## Container-aware rollouts

Migrations run inside the API container. After committing a migration, the rebuild may need `--no-cache` (project rule: `--no-cache` only when migrations changed). Confirm no parallel session is mid-build before triggering.
