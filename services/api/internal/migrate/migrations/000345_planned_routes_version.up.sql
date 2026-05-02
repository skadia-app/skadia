-- Optimistic-concurrency version column for planned_routes.
-- Used as the strong-ETag source: every UPDATE bumps `version = version + 1`,
-- and PATCH/PUT requests carry the previously-observed value via `If-Match`.
-- A WHERE-clause mismatch (RowsAffected = 0) translates to HTTP 412.
--
-- Default 1 (not 0): existing rows must be readable before any concurrent
-- write happens, and 0-as-default would make every legacy row trivially
-- mismatch the absent-or-fresh ETag a client could observe.
ALTER TABLE planned_routes
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

COMMENT ON COLUMN planned_routes.version IS
    'Optimistic-concurrency token (RFC 7232 strong ETag source). '
    'Incremented by every UPDATE; PATCH callers send it via If-Match.';

-- No new index: planned_routes.id is the PK, and the WHERE clause for the
-- guarded UPDATE (`id = $1 AND user_id = $2 AND version = $3`) hits the PK
-- index plus an in-row predicate. Adding a composite index on (id, version)
-- would duplicate PK coverage with no measurable benefit.
