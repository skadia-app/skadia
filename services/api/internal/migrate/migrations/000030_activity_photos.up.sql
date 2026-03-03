-- Fotos zu Aktivitäten (P2-Feature)
CREATE TABLE activity_photos (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id      UUID REFERENCES activities(id) ON DELETE SET NULL,
    s3_bucket        TEXT NOT NULL,
    s3_key           TEXT NOT NULL,
    s3_key_thumb     TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    content_type     TEXT NOT NULL,
    file_size_bytes  BIGINT NOT NULL,
    file_hash        TEXT,
    taken_at         TIMESTAMPTZ,
    lat              DOUBLE PRECISION,
    lng              DOUBLE PRECISION,
    location_source  TEXT CHECK (location_source IN ('exif', 'activity_track')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON activity_photos (user_id);
CREATE INDEX ON activity_photos (activity_id);
CREATE INDEX ON activity_photos (user_id) WHERE lat IS NOT NULL;
