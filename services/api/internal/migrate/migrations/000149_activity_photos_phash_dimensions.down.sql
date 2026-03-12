ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS phash_threshold_bits;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS height;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS width;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS phash;
