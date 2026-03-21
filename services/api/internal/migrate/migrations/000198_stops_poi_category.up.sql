ALTER TABLE activity_stops
    ADD COLUMN IF NOT EXISTS poi_category TEXT,
    ADD COLUMN IF NOT EXISTS confidence TEXT DEFAULT 'poi_matched';
