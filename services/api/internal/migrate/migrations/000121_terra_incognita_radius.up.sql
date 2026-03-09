-- User preference for terra incognita buffer radius in meters (default 1000m).
ALTER TABLE user_enrichment_settings
    ADD COLUMN terra_incognita_radius_m INTEGER NOT NULL DEFAULT 1000;
