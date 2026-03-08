-- Expand health_snapshots with all fields from Garmin Health Snapshot feature.
-- Previously only rmssd_ms/sdrr_ms were stored; HR, SpO2, respiration, stress were lost.
ALTER TABLE health_snapshots
    ADD COLUMN IF NOT EXISTS avg_heart_rate_bpm SMALLINT,
    ADD COLUMN IF NOT EXISTS spo2_pct           SMALLINT,
    ADD COLUMN IF NOT EXISTS respiration_brpm   SMALLINT,
    ADD COLUMN IF NOT EXISTS stress_level       SMALLINT,
    ADD COLUMN IF NOT EXISTS title              TEXT;
