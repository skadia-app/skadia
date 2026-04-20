ALTER TABLE user_enrichment_settings
    ADD COLUMN summit_detection_radius_m REAL NOT NULL DEFAULT 80.0
    CHECK (summit_detection_radius_m BETWEEN 10 AND 150);
