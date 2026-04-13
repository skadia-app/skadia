-- Migration 000216 created a UNIQUE INDEX named uq_activity_time_in_zone_v2,
-- but ON CONFLICT ON CONSTRAINT requires a named CONSTRAINT, not just an index.
-- This migration registers the existing index as a proper unique constraint so
-- that the ON CONFLICT ON CONSTRAINT clause in SaveTimeInZones works correctly.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_activity_time_in_zone_v2'
          AND conrelid = 'activity_time_in_zone'::regclass
    ) THEN
        ALTER TABLE activity_time_in_zone
            ADD CONSTRAINT uq_activity_time_in_zone_v2
            UNIQUE USING INDEX uq_activity_time_in_zone_v2;
    END IF;
END $$;
