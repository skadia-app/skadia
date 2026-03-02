-- Heatmap-Points: Punkt-basierte Heatmap fuer Martin Tile Server

CREATE TABLE heatmap_points (
    id            BIGSERIAL PRIMARY KEY,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id   UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    sport_type    TEXT,
    activity_date DATE,
    geom          GEOMETRY(Point, 4326) NOT NULL
);

CREATE INDEX ON heatmap_points USING GIST (geom);
CREATE INDEX ON heatmap_points (user_id, sport_type);
CREATE INDEX ON heatmap_points (activity_id);

-- User-Einstellung fuer Sampling-Intervall
ALTER TABLE users ADD COLUMN IF NOT EXISTS heatmap_sample_interval_m INTEGER DEFAULT 20;
