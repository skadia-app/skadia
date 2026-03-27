ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS percent_hydration   REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS visceral_fat_mass_g INTEGER;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS basal_met           REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS active_met          REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS physique_rating     SMALLINT;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS metabolic_age       SMALLINT;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS visceral_fat_rating SMALLINT;
