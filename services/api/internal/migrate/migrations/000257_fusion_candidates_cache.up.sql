-- Cache-Tabelle
CREATE TABLE fusion_candidates (
    user_id         TEXT NOT NULL,
    activity_id_a   UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    activity_id_b   UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    overlap_percent DOUBLE PRECISION NOT NULL,
    field_richness_a INT NOT NULL DEFAULT 0,
    field_richness_b INT NOT NULL DEFAULT 0,
    time_diff_sec   DOUBLE PRECISION NOT NULL DEFAULT 0,
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, activity_id_a, activity_id_b)
);

-- Sequence-Counter für Invalidierung
ALTER TABLE users ADD COLUMN fusion_invalidation_seq BIGINT NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN fusion_computed_seq BIGINT NOT NULL DEFAULT 0;
-- Default 1 vs 0 → alle bestehenden User = stale → automatischer Backfill

-- Trigger-Funktion
CREATE OR REPLACE FUNCTION invalidate_fusion_cache() RETURNS trigger AS $$
DECLARE
    affected_user TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_user := OLD.user_id;
    ELSE
        affected_user := NEW.user_id;
    END IF;
    UPDATE users SET fusion_invalidation_seq = fusion_invalidation_seq + 1
    WHERE id = affected_user;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;

-- INSERT/DELETE: immer feuern
CREATE TRIGGER trg_fusion_cache_ins_del
    AFTER INSERT OR DELETE ON activities
    FOR EACH ROW EXECUTE FUNCTION invalidate_fusion_cache();

-- UPDATE: NUR bei fusions-relevanten Spalten feuern
CREATE TRIGGER trg_fusion_cache_upd
    AFTER UPDATE ON activities
    FOR EACH ROW
    WHEN (OLD.started_at IS DISTINCT FROM NEW.started_at
       OR OLD.ended_at IS DISTINCT FROM NEW.ended_at
       OR OLD.has_gps IS DISTINCT FROM NEW.has_gps
       OR OLD.is_fusion_source IS DISTINCT FROM NEW.is_fusion_source
       OR OLD.fusion_sources IS DISTINCT FROM NEW.fusion_sources)
    EXECUTE FUNCTION invalidate_fusion_cache();
