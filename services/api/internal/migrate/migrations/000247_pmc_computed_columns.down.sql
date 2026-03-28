ALTER TABLE activities
  DROP COLUMN IF EXISTS computed_tss,
  DROP COLUMN IF EXISTS computed_np_w,
  DROP COLUMN IF EXISTS computed_ngp_mps,
  DROP COLUMN IF EXISTS tss_method,
  DROP COLUMN IF EXISTS computed_if;

ALTER TABLE training_zones
  DROP COLUMN IF EXISTS threshold_speed_mps;

ALTER TABLE pmc_daily
  DROP COLUMN IF EXISTS acwr_ra;
