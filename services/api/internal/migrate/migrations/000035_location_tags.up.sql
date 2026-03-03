-- Add geo-coordinates to tags so a tag can optionally act as a "location tag"
-- (a regular tag that also appears as a pin on the map).
ALTER TABLE tags
    ADD COLUMN lat DOUBLE PRECISION,
    ADD COLUMN lng DOUBLE PRECISION;

-- Partial index for fast lookup of location-tags per user
CREATE INDEX ON tags (user_id) WHERE lat IS NOT NULL AND lng IS NOT NULL;
