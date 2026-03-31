-- Recording Groups: Fasst N Aufnahmen desselben Events zusammen
CREATE TABLE recording_groups (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    primary_activity_id UUID REFERENCES activities(id) ON DELETE SET NULL,
    sport_type          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed           BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE recording_group_members (
    group_id    UUID NOT NULL REFERENCES recording_groups(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    is_fused    BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (group_id, activity_id),
    UNIQUE (activity_id)
);

CREATE INDEX idx_recording_groups_user ON recording_groups (user_id);
CREATE INDEX idx_recording_group_members_activity ON recording_group_members (activity_id);

-- Statistik-Exklusivität: Nur primäre Aktivität zählt
ALTER TABLE activities ADD COLUMN IF NOT EXISTS is_group_secondary BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_activities_group_secondary ON activities (is_group_secondary) WHERE is_group_secondary = TRUE;

-- Trigger: is_group_secondary synchron halten
CREATE OR REPLACE FUNCTION fn_sync_group_secondary() RETURNS TRIGGER AS $$
DECLARE
    v_group_id UUID;
    v_primary UUID;
    v_member_count INT;
BEGIN
    -- Guard gegen rekursive Trigger-Aufrufe (CASCADE kann denselben Trigger feuern)
    IF pg_trigger_depth() > 1 THEN
        RETURN COALESCE(NEW, OLD);
    END IF;

    -- Bestimme die betroffene Gruppe
    IF TG_TABLE_NAME = 'recording_group_members' THEN
        IF TG_OP = 'DELETE' THEN
            v_group_id := OLD.group_id;
        ELSE
            v_group_id := NEW.group_id;
        END IF;
    ELSIF TG_TABLE_NAME = 'recording_groups' THEN
        v_group_id := NEW.id;
    END IF;

    -- Zähle verbleibende Mitglieder
    SELECT COUNT(*) INTO v_member_count
    FROM recording_group_members WHERE group_id = v_group_id;

    -- Weniger als 2 Mitglieder → Gruppe auflösen
    IF v_member_count < 2 THEN
        UPDATE activities SET is_group_secondary = FALSE
        WHERE id IN (SELECT activity_id FROM recording_group_members WHERE group_id = v_group_id);
        DELETE FROM recording_groups WHERE id = v_group_id;
        RETURN COALESCE(NEW, OLD);
    END IF;

    -- Primäre Aktivität bestimmen
    SELECT primary_activity_id INTO v_primary
    FROM recording_groups WHERE id = v_group_id;

    -- Falls primary NULL (z.B. nach ON DELETE SET NULL) → neu wählen
    IF v_primary IS NULL THEN
        SELECT rgm.activity_id INTO v_primary
        FROM recording_group_members rgm
        JOIN activities a ON a.id = rgm.activity_id
        WHERE rgm.group_id = v_group_id
        ORDER BY (
            CASE WHEN a.avg_heart_rate IS NOT NULL THEN 1 ELSE 0 END +
            CASE WHEN a.avg_power_w IS NOT NULL THEN 1 ELSE 0 END +
            CASE WHEN a.has_gps THEN 1 ELSE 0 END +
            CASE WHEN a.avg_temperature_c IS NOT NULL THEN 1 ELSE 0 END +
            CASE WHEN a.avg_cadence IS NOT NULL THEN 1 ELSE 0 END +
            CASE WHEN a.elevation_gain_m IS NOT NULL THEN 1 ELSE 0 END
        ) DESC, a.created_at ASC
        LIMIT 1;

        UPDATE recording_groups SET primary_activity_id = v_primary WHERE id = v_group_id;
    END IF;

    -- is_group_secondary für alle Mitglieder setzen
    UPDATE activities SET is_group_secondary = TRUE
    WHERE id IN (SELECT activity_id FROM recording_group_members WHERE group_id = v_group_id)
      AND id != v_primary;

    UPDATE activities SET is_group_secondary = FALSE
    WHERE id = v_primary;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- CONSTRAINT TRIGGER: Feuert erst bei COMMIT, nicht nach jedem einzelnen INSERT.
-- Ohne DEFERRABLE würde der Trigger beim ersten Member-INSERT count=1 finden
-- und die Gruppe sofort löschen, bevor der zweite Member eingefügt wird.
CREATE CONSTRAINT TRIGGER trg_group_members_sync
    AFTER INSERT OR DELETE ON recording_group_members
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION fn_sync_group_secondary();

-- Primary-Update Trigger: Normaler AFTER-Trigger (nicht deferred nötig,
-- da primary_activity_id nur einzeln geändert wird).
CREATE TRIGGER trg_group_primary_sync
    AFTER UPDATE OF primary_activity_id ON recording_groups
    FOR EACH ROW EXECUTE FUNCTION fn_sync_group_secondary();
