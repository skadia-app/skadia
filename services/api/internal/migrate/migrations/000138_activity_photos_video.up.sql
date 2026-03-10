-- Migration 000138: Video-Metadaten für activity_photos (mp4, mov, avi, mkv, webm)

ALTER TABLE activity_photos
    ADD COLUMN IF NOT EXISTS is_video BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS duration_s NUMERIC,      -- Video-Länge in Sekunden
    ADD COLUMN IF NOT EXISTS video_width INT,
    ADD COLUMN IF NOT EXISTS video_height INT;
