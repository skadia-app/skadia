ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS subcutaneous_fat_pct REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS protein_pct REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS skeletal_muscle_pct REAL;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS lean_body_mass_g INTEGER;
ALTER TABLE health_weight_entries ADD COLUMN IF NOT EXISTS total_body_water_l REAL;
