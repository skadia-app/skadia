ALTER TABLE users
  ADD COLUMN IF NOT EXISTS birth_date              DATE,
  ADD COLUMN IF NOT EXISTS gender                  TEXT DEFAULT '' CHECK (gender IN ('', 'M', 'F', 'O')),
  ADD COLUMN IF NOT EXISTS height_cm               SMALLINT,
  ADD COLUMN IF NOT EXISTS weight_kg               DECIMAL(5,1),
  ADD COLUMN IF NOT EXISTS ftp_w                   INTEGER,
  ADD COLUMN IF NOT EXISTS lthr_bpm                INTEGER,
  ADD COLUMN IF NOT EXISTS max_hr_bpm              INTEGER,
  ADD COLUMN IF NOT EXISTS max_hr_detected_bpm     INTEGER,
  ADD COLUMN IF NOT EXISTS resting_hr_bpm          INTEGER;
-- max_hr_formula_bpm is computed in Go (Tanaka: 208 - 0.7 × age) — AGE() is not immutable,
-- so it cannot be a GENERATED ALWAYS AS STORED column in PostgreSQL.
