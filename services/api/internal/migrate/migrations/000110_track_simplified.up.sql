-- activity_tracks.track_simplified existiert bereits (GEOMETRY(LineString, 4326)).
-- Diese Migration stellt nur sicher, dass der GIST-Index vorhanden ist.
CREATE INDEX IF NOT EXISTS idx_activity_tracks_track_simplified
    ON activity_tracks USING GIST (track_simplified);
