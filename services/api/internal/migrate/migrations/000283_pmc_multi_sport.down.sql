DROP INDEX IF EXISTS idx_pmc_daily_sport;
-- Remove sport-specific rows before restoring original PK
DELETE FROM pmc_daily WHERE sport_group != '__combined__';
ALTER TABLE pmc_daily DROP CONSTRAINT pmc_daily_pkey;
ALTER TABLE pmc_daily ADD PRIMARY KEY (user_id, date);
ALTER TABLE pmc_daily DROP COLUMN sport_group;
