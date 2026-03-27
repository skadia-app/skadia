ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_light          SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_awake_time     SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_awakenings     SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_combined_awake SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_restlessness   SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_interruptions  SMALLINT;
ALTER TABLE health_sleep ADD COLUMN IF NOT EXISTS score_recovery       SMALLINT;
