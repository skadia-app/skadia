ALTER TABLE health_snapshots
    DROP COLUMN IF EXISTS avg_heart_rate_bpm,
    DROP COLUMN IF EXISTS spo2_pct,
    DROP COLUMN IF EXISTS respiration_brpm,
    DROP COLUMN IF EXISTS stress_level,
    DROP COLUMN IF EXISTS title;
