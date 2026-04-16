-- Phase 4b revert safety — track photo origin so revert can restore activity_id.
-- When FusionMerge reassigns photos from source activities to the merged activity,
-- source_activity_id records where each photo came from. RevertFusion uses this to
-- restore photos to their original source before DeleteActivity triggers CASCADE.
BEGIN;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS source_activity_id UUID NULL REFERENCES activities(id) ON DELETE SET NULL;
-- Partial index for revert path: fast lookup of reassigned-to-merged photos by source.
CREATE INDEX IF NOT EXISTS idx_activity_photos_source_activity_id ON activity_photos (source_activity_id) WHERE source_activity_id IS NOT NULL;
COMMIT;
