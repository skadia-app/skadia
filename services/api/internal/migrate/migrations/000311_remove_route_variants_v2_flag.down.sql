-- A rollback of this migration is not purely schema-level: the Up-Migration
-- removed a server_settings row that only existed because the corresponding
-- Go KeyDef was registered in settings/keys.go. Re-inserting the row without
-- reverting the code commits that removed the KeyDef would create an
-- inconsistent state (DB has the row, code doesn't read it anymore).
--
-- A correct rollback therefore requires BOTH:
--   1. Revert the code commits that removed the KeyDef + handler logic.
--   2. Manually re-insert the server_settings row after the code revert:
--        INSERT INTO server_settings (key, value, updated_at)
--          VALUES ('routing.route_variants_v2', 'false', now());
--
-- To prevent accidental "silent-success" rollbacks, this down-migration
-- deliberately fails with a descriptive exception. The operator must
-- acknowledge the manual steps before proceeding.
DO $$ BEGIN
  RAISE EXCEPTION 'irreversible migration via automated rollback: restoring routing.route_variants_v2 requires reverting code commits (see 000311 down.sql comment for the manual INSERT statement).';
END $$;
