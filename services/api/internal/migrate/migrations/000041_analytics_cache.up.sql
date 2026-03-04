-- 000041_analytics_cache.up.sql

-- Vorberechnete Analytics-Daten (ML-Insights, Eddington, Streaks)
CREATE TABLE IF NOT EXISTS analytics_cache (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cache_key   TEXT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    data        JSONB NOT NULL,
    PRIMARY KEY (user_id, cache_key)
);

CREATE INDEX idx_analytics_cache_expires ON analytics_cache(expires_at)
    WHERE expires_at IS NOT NULL;

-- Commute- und Wettkampf-Flags für Activities-Filter
ALTER TABLE activities ADD COLUMN IF NOT EXISTS is_commute BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS is_race BOOLEAN NOT NULL DEFAULT FALSE;

-- R-R Intervalle für DFA-alpha1 HRV-Analyse
CREATE TABLE IF NOT EXISTS activity_rr_intervals (
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    rr_ms       SMALLINT NOT NULL,
    PRIMARY KEY (activity_id, seq)
);
