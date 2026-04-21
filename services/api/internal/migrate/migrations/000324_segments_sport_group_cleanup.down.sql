-- 000324_segments_sport_group_cleanup.down.sql
-- Down ist NICHT rückführbar — originale sport_group-Werte verloren.
-- RAISE EXCEPTION verhindert silent-success + Version-Counter-Dekrement.
DO $$ BEGIN
  RAISE EXCEPTION 'Migration 000324 down not supported: sport_group cleanup is irreversible. Restore from DB backup required.';
END $$;
