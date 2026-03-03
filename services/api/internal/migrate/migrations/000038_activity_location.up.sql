-- Manuell zugewiesener Standort für Aktivitäten ohne GPS-Daten (Indoor-Tagging).
-- Getrennt von start_lat/start_lng, die ausschließlich GPS-Track-Daten enthalten.
ALTER TABLE activities
    ADD COLUMN location_lat DOUBLE PRECISION,
    ADD COLUMN location_lng DOUBLE PRECISION;

COMMENT ON COLUMN activities.location_lat IS 'Manually assigned location from a location tag (distinct from GPS-derived start_lat)';
COMMENT ON COLUMN activities.location_lng IS 'Manually assigned location from a location tag (distinct from GPS-derived start_lng)';
