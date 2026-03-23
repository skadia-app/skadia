-- 1. Tabelle erstellen falls nicht vorhanden
CREATE TABLE IF NOT EXISTS activity_time_in_zone (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    zone_type TEXT NOT NULL,
    zone_index INTEGER NOT NULL,
    zone_high DOUBLE PRECISION,
    seconds INTEGER
);

-- 2. Neue Spalten hinzufügen (IF NOT EXISTS für Idempotenz)
ALTER TABLE activity_time_in_zone ADD COLUMN IF NOT EXISTS lap_id UUID REFERENCES activity_laps(id) ON DELETE CASCADE;
ALTER TABLE activity_time_in_zone ADD COLUMN IF NOT EXISTS segment_lap_id UUID REFERENCES activity_segment_laps(id) ON DELETE CASCADE;

-- 3. Alten UNIQUE Constraint/Index finden und droppen
DO $$
DECLARE
    idx_name TEXT;
BEGIN
    -- Suche nach existierendem UNIQUE constraint oder index
    SELECT indexname INTO idx_name
    FROM pg_indexes
    WHERE tablename = 'activity_time_in_zone'
    AND indexdef LIKE '%UNIQUE%'
    LIMIT 1;

    IF idx_name IS NOT NULL THEN
        EXECUTE format('DROP INDEX IF EXISTS %I', idx_name);
    END IF;
END $$;

-- 4. Neuer UNIQUE Index mit COALESCE (exakt aus Spec Section 5.1)
CREATE UNIQUE INDEX IF NOT EXISTS uq_activity_time_in_zone_v2 ON activity_time_in_zone (
    activity_id, zone_type, zone_index,
    COALESCE(lap_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(segment_lap_id, '00000000-0000-0000-0000-000000000000'::uuid)
);

-- 5. Indexes für lap/segment FK Lookups
CREATE INDEX IF NOT EXISTS idx_time_in_zone_lap ON activity_time_in_zone(lap_id) WHERE lap_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_time_in_zone_segment_lap ON activity_time_in_zone(segment_lap_id) WHERE segment_lap_id IS NOT NULL;
