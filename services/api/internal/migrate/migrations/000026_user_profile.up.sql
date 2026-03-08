-- Add physiological profile columns to users table.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS birth_date          DATE,
  ADD COLUMN IF NOT EXISTS gender              TEXT,
  ADD COLUMN IF NOT EXISTS height_cm           INTEGER,
  ADD COLUMN IF NOT EXISTS weight_kg           REAL,
  ADD COLUMN IF NOT EXISTS ftp_w               INTEGER,
  ADD COLUMN IF NOT EXISTS lthr_bpm            INTEGER,
  ADD COLUMN IF NOT EXISTS max_hr_bpm          INTEGER,
  ADD COLUMN IF NOT EXISTS max_hr_detected_bpm INTEGER,
  ADD COLUMN IF NOT EXISTS resting_hr_bpm      INTEGER;
