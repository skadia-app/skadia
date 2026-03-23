-- 1. Neue Spalten hinzufügen (IF NOT EXISTS für Idempotenz)
-- Tabelle existiert bereits mit PK (activity_id, zone_type, zone_index)
ALTER TABLE activity_time_in_zone ADD COLUMN IF NOT EXISTS lap_id UUID REFERENCES activity_laps(id) ON DELETE CASCADE;
ALTER TABLE activity_time_in_zone ADD COLUMN IF NOT EXISTS segment_lap_id UUID REFERENCES activity_segment_laps(id) ON DELETE CASCADE;

-- 2. Alten PRIMARY KEY oder UNIQUE Constraint droppen
-- Die bestehende Tabelle hat einen composite PK (activity_id, zone_type, zone_index)
-- der inkompatibel mit lap-level Zonen ist. PK muss via ALTER TABLE DROP CONSTRAINT
-- entfernt werden (nicht DROP INDEX).
DO $$
DECLARE
    con_name TEXT;
BEGIN
    -- Erst PK-Constraint suchen
    SELECT conname INTO con_name
    FROM pg_constraint
    WHERE conrelid = 'activity_time_in_zone'::regclass
    AND contype = 'p';

    IF con_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE activity_time_in_zone DROP CONSTRAINT %I', con_name);
    END IF;

    -- Dann beliebige UNIQUE Constraints/Indexes suchen
    FOR con_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'activity_time_in_zone'::regclass
        AND contype = 'u'
    LOOP
        EXECUTE format('ALTER TABLE activity_time_in_zone DROP CONSTRAINT %I', con_name);
    END LOOP;
END $$;

-- 3. Surrogat-ID hinzufügen (bestehende Rows bekommen generierte UUIDs)
ALTER TABLE activity_time_in_zone ADD COLUMN IF NOT EXISTS id UUID DEFAULT gen_random_uuid();
UPDATE activity_time_in_zone SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE activity_time_in_zone ALTER COLUMN id SET NOT NULL;

-- 4. Neuer UNIQUE Index mit COALESCE (ersetzt den alten PK)
CREATE UNIQUE INDEX IF NOT EXISTS uq_activity_time_in_zone_v2 ON activity_time_in_zone (
    activity_id, zone_type, zone_index,
    COALESCE(lap_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(segment_lap_id, '00000000-0000-0000-0000-000000000000'::uuid)
);

-- 5. Indexes für lap/segment FK Lookups
CREATE INDEX IF NOT EXISTS idx_time_in_zone_lap ON activity_time_in_zone(lap_id) WHERE lap_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_time_in_zone_segment_lap ON activity_time_in_zone(segment_lap_id) WHERE segment_lap_id IS NOT NULL;
