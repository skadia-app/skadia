DROP INDEX IF EXISTS idx_activity_photos_clip_embedding;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS clip_embedding;
ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS enable_photo_clip_embedding;
