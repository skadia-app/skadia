DROP INDEX IF EXISTS idx_time_in_zone_segment_lap;
DROP INDEX IF EXISTS idx_time_in_zone_lap;
DROP INDEX IF EXISTS uq_activity_time_in_zone_v2;

DO $$
BEGIN
    IF to_regclass('public.activity_time_in_zone') IS NULL THEN
        RETURN;
    END IF;

    ALTER TABLE activity_time_in_zone DROP COLUMN IF EXISTS id;
    ALTER TABLE activity_time_in_zone DROP COLUMN IF EXISTS segment_lap_id;
    ALTER TABLE activity_time_in_zone DROP COLUMN IF EXISTS lap_id;

    IF to_regclass('public.uq_activity_time_in_zone') IS NULL THEN
        EXECUTE 'CREATE UNIQUE INDEX uq_activity_time_in_zone ON activity_time_in_zone (activity_id, zone_type, zone_index)';
    END IF;
END $$;
