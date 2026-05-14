# services/api/internal/migrate — Agent Instructions

See [`../../../../AGENTS.md`](../../../../AGENTS.md) (project-wide), [`../../AGENTS.md`](../../AGENTS.md) (backend), and [`README.md`](README.md) (operational runbook: rollback ordering, Fusion-refactor 000302–308 strict order, 000308 failure recovery — read it before touching anything in that range).

## The only migrations directory

`migrations/` here is the single source of truth — no other migrations dir exists. Don't look for or create one elsewhere.

## Naming + structure

`NNNNNN_short_snake_case_description.{up,down}.sql`

- 6-digit zero-padded number, monotonically increasing.
- MUST ship both `.up.sql` and `.down.sql`. The `.down.sql` must actually undo the `.up.sql` (verify locally where feasible).
- Optional matching test inside `migrations/`: `migration_NNNNNN_test.go` (see `migrations/migration_000326_test.go`).

## Branch numbering and finalization

Branch migration numbers are provisional until `scripts/finalize-migration.sh`
normalizes them against `BASE_REF` (default `main`); another branch may merge
first.

- Use unique high provisional numbers, e.g. `900001_<topic>`, `900002_<next>`.
- Multiple branch migrations must never share one provisional number.
- Test names use `TestMigration_<TopicCamelCase>_*`, never numeric names.
- Plan text references the topic, not the provisional number.
- Before merge:
  `./scripts/finalize-migration.sh`;
  `BASE_REF=main ./scripts/finalize-migration.sh --check`;
  `./scripts/test_db.sh run -- sh -c 'cd services/api && go test ./internal/migrate/...'`.
- The script scans branch-only migrations, assigns sequential base-branch
  numbers, `git mv`s files, patches code/doc references, and CI blocks
  unfinalized numbers.

On single-branch hotfixes directly on `main`, explicit user approval can skip
provisional numbering; still `ls migrations/` before assigning and never trust a
remembered next free number.

## Session-variable namespaces

When introducing a Postgres `SET LOCAL` / `current_setting` key:

- RLS context → `app.*` (e.g. `app.current_user_id`).
- Anything else (feature flags, trigger-skip gates) → `skadia.*` (e.g. `skadia.skip_rgm_cleanup`).

## GDPR

Migration adds a table with a user_id reference (direct or via FK chain) → same
commit must update the GDPR registry; follow the GDPR checklist in
[`../../AGENTS.md`](../../AGENTS.md).

## Column drops

Before any `DROP COLUMN`: `grep -r "<column>"` across the repo (Go, SQL, TS). Hits outside this migration mean callers exist — coordinate with the user.

## Container-aware rollouts

Migrations run inside the API container. After committing a migration, the rebuild may need `--no-cache` (project rule: `--no-cache` only when migrations changed). Confirm no parallel session is mid-build before triggering.
