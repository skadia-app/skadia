-- Phase 4b.A.1 — System-Artefakt-Architektur entkoppeln.
--
-- 1. Validation moves into Go (services/api/internal/automation/system_artifacts.go).
--    Rationale: every prior system_type addition required a migration just to
--    expand the CHECK whitelist. Code-level validation makes adding a new
--    system rule a single-file Go change.
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;

-- 2. Versions-Gating für EnsureSystemArtifacts.
--    Without this column the startup hook iterates O(N) users on every pod
--    start. With the column O(new + changed users) — relevant for any future
--    multi-instance deployment and even on single-instance selfhosts once
--    user counts exceed ~10k.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS system_artifacts_version INT NOT NULL DEFAULT 0;

-- 3. BTree-Index on system_artifacts_version. Intentionally NOT a partial index:
--    the sweep filters by "WHERE system_artifacts_version < $current_version",
--    but $current_version is the Go-constant updated on every spec change, not
--    a static SQL predicate. A partial index with a hard sentinel (e.g.
--    < 1000000) would effectively be a full index until version 1M is reached.
--    Plain BTree gives the planner a selective range scan with monotonically
--    increasing versions.
CREATE INDEX IF NOT EXISTS idx_users_system_artifacts_version
  ON users (system_artifacts_version);
