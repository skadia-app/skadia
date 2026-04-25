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

## GDPR

Any new table with a user_id reference (direct or via FK chain) → all 5 steps from the project-wide GDPR checklist in [`../../AGENTS.md`](../../AGENTS.md) (section: GDPR for new tables). CI test `gdpr_registry_test.go` fails if missing from `internal/auth/gdpr_registry.go`.

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

`internal/routing/` is for outdoor profiles (foot, bike, ski, …) — not street/car routing. See **Routing mindset** in [`../../AGENTS.md`](../../AGENTS.md) for the cost-function principles.
