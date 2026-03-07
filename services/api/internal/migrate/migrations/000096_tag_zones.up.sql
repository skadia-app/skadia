CREATE TABLE tag_zones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tag_id      UUID NOT NULL UNIQUE REFERENCES tags(id) ON DELETE CASCADE,
    zone_type   TEXT NOT NULL CHECK (zone_type IN ('circle', 'polygon')),
    center      GEOMETRY(Point, 4326),
    radius_m    DOUBLE PRECISION,
    polygon     GEOMETRY(Polygon, 4326),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tag_zones_polygon ON tag_zones USING GIST(polygon);
CREATE INDEX idx_tag_zones_tag_id ON tag_zones(tag_id);

COMMENT ON TABLE tag_zones IS 'Geo-zones for location tags (1:1 with tags that have zones)';
COMMENT ON COLUMN tag_zones.zone_type IS 'circle or polygon';
COMMENT ON COLUMN tag_zones.center IS 'Pin position (exact point, used for proximity checks in automation rules)';
COMMENT ON COLUMN tag_zones.radius_m IS 'Only for circle zones';
COMMENT ON COLUMN tag_zones.polygon IS 'Zone polygon, may contain inner rings (holes)';
