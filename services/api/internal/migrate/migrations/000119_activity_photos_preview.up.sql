-- Add s3_key_preview for the large preview thumbnail (2048 / 4K) per photo.
-- Empty string means no preview was generated (original used as fallback for display).
ALTER TABLE activity_photos ADD COLUMN s3_key_preview TEXT NOT NULL DEFAULT '';
