# services/api — Agent Instructions

See [`../../AGENTS.md`](../../AGENTS.md) for project-wide rules. Go-backend specifics below.

## Layout

- Entry point: `cmd/api/`.
- Internal packages: `internal/<domain>/` (e.g. `auth`, `database`, `enrichment`, `fusion`, `routing`, `migrate`).
- Migrations: **only** `internal/migrate/migrations/` — see [`internal/migrate/AGENTS.md`](internal/migrate/AGENTS.md).

## Go 1.26 — required idioms

`go.mod` pins Go 1.26. If your training cutoff predates this, the patterns below are mandatory — `go fix ./...` will rewrite many of them automatically, but new code must already be in this style. Background: ADR-0016 (`docs/documentation/dev/adr/0016-go-1-26-adoption.md`); deeper reference: `docs/_internal/go-conventions.md`.

- `any` not `interface{}` (Go 1.18+, enforced by `go fix`).
- `for i := range n` not `for i := 0; i < n; i++` (Go 1.22+, enforced by `go fix`).
- Loop variables are per-iteration since Go 1.22 — closures over `range` vars are safe; **don't** insert the old `v := v` shadow.
- `wg.Go(func(){ ... })` not `wg.Add(1) + go func(){ defer wg.Done(); ... }()` (Go 1.25).
- `errors.AsType[T](err)` not `errors.As(err, &target)` (Go 1.26).
- `slices.Contains`, `slices.Index`, `slices.Reverse`, `slices.Collect(maps.Keys(m))` — never hand-rolled loops.
- `min(a, b)` / `max(a, b, c)` are **builtins** (Go 1.21+) — don't define helpers.
- `bytes.Buffer.Peek(n)`, `reflect.Type.Fields()` iterator, `slog.NewMultiHandler` — Go 1.26 stdlib, **not unknown functions**; don't flag them in review.
- Hot-path strings: `strings.Builder` + `strconv.FormatFloat/Itoa` over `fmt.Sprintf`; pre-grow with `sb.Grow(n)`.
- External `io.ReadAll(resp.Body)` → wrap with `io.LimitReader` and a sane cap.
- Run `go fix ./...` after any larger Go change.

## Tests

- Integration tests use `TEST_DATABASE_URL` (NOT `DATABASE_URL`). Skip with `t.Skip("TEST_DATABASE_URL not set")` if unset.
- **Test helpers are file-scoped, not shared.** No project-wide `newTestPool` / `insertRecordingGroup`. Each `_test.go` defines its own setup helper (named like `*TestPool`). Before referencing any helper in a plan/task, `grep` for `func.*TestPool` in the package and use the actual one — or add a new file-scoped helper. Never invent shared helpers.
- Before writing `SomeStruct{Field: ...}`, `Read` the struct definition. Field names like `RecordingGroupID` vs `GroupID` matter; missing required fields silently mis-compile.

## Verify backend reality

- Assuming `DATABASE_URL` instead of grepping tests for `TEST_DATABASE_URL` is a known failure mode.
- Writing `FusionMergeRequest{RecordingGroupID: ...}` without reading the struct is wrong; the real field is `GroupID`, plus required fields.
- Verify helper/function/struct names and current `path:line` anchors against this checkout; do not cite remembered lines such as `fusion_merge.go:601`.
- Recompute LOC/function counts with `wc -l` / `grep -c '^func'`; do not reuse remembered counts.

## Test DB per worktree

- Parallel worktrees must not share `skadia_test`: divergent migrations overwrite schema state, provisional numbers collide, and the runner trusts highest `schema_migrations`.
- Each worktree uses `skadia_test_<slug>` via `scripts/test_db.sh`.
- Full rebuild means `DROP DATABASE` + `CREATE DATABASE` + `migrate up`, never `DROP SCHEMA public CASCADE`.
- Migration fingerprints include `(relative_path + file_content)` for every migration and migration test file.
- Tests use `TEST_DATABASE_URL` or `services/api/internal/database/testdb.Open(t)`; uncontrolled `os.Getenv("DATABASE_URL")` is forbidden by `testdb_safety_test.go`.
- Per new shell: `./scripts/test_db.sh ensure`; `eval "$(./scripts/test_db.sh emit-url)"`; then run tests, or use `./scripts/test_db.sh run -- sh -c 'cd services/api && go test ./...'`.
- Cleanup/inspect: `./scripts/test_db.sh gc`; `./scripts/test_db.sh inspect`.
- Details: `docs/documentation/dev/subsystems/test-db.md`, `docs/documentation/dev/adr/0037-test-db-per-worktree.md`.

## GDPR

Any new table with a `user_id` reference, direct or via FK chain:

1. Register in `internal/auth/gdpr_registry.go`.
2. If Export/External, add `exportTable()` in `internal/auth/handler_data_export.go`.
3. Verify CASCADE or add deletion in `internal/database/gdpr.go`.
4. Add security-sensitive columns to `ExportDenyColumns`.
5. `gdpr_registry_test.go` must pass.

## Session variables

- `app.*` → RLS context (e.g. `app.current_user_id`).
- `skadia.*` → feature flags + trigger-skip gates.

New project-owned session variables → `skadia.*` unless RLS context.

## Column drops

Before any DB column drop: `grep` the entire repo (Go, SQL, frontend types). Hits outside the migration = unsafe — coordinate with the user.

## SQL NULL semantics

Any WHERE refactor (`a = b` → `a IS NOT DISTINCT FROM b`, negations, JOIN conditions) on nullable columns must be verified against the 5-case truth table before merging:

1. `a IS NULL AND b IS NULL` (both null)
2. `a IS NULL AND b IS NOT NULL`
3. `a IS NOT NULL AND b IS NULL`
4. `a = b AND a IS NOT NULL` (both set, equal)
5. `a != b AND a IS NOT NULL AND b IS NOT NULL` (both set, different)

`NULL = NULL` is `UNKNOWN`, not `TRUE`. When unsure: `SELECT (NULL = NULL);` in psql.

## Query-shape changes need analysis, not dispatch

Expanding a hot-path query (e.g. `GetStreamPoints` from 10 to 80+ columns) is **not** a Standard-tier agent task. Before touching: estimate response size (cols × rows), verify TimescaleDB compression behavior with the wider SELECT, check downstream consumers, and decide whether selective loading suffices. Reasoning-tier analysis first; agent dispatch second.

## Outdoor routing

Routing is part of the API container. These rules apply to routing code and to
plans/specs for router behavior, even when the plan file itself lives outside
`services/api/`.

- Verify whether the change targets BRouter, BadRouter, or both.
- Skadia routing is outdoor routing, not street/car routing: elevation,
  surface, traffic, and noise weights differ from street-router heuristics.
- Harder terrain is not always more expensive; some bike profiles may prefer
  unpaved surfaces.
- Noise and traffic penalties scale with surface and population density, not
  linearly with road class.
- Elevation gain is per-segment work; a flat detour can beat a steep shortcut.
- Surface preferences are profile-specific; never hard-code one global ranking.
