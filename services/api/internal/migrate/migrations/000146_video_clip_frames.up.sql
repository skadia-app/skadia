ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS video_clip_frames INTEGER NOT NULL DEFAULT 1
        CHECK (video_clip_frames BETWEEN 1 AND 10);
