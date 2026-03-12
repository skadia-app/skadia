ALTER TABLE activity_photos ADD COLUMN phash BIGINT;
ALTER TABLE activity_photos ADD COLUMN width INTEGER;
ALTER TABLE activity_photos ADD COLUMN height INTEGER;
ALTER TABLE user_enrichment_settings ADD COLUMN phash_threshold_bits INTEGER NOT NULL DEFAULT 3;
