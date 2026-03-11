ALTER TABLE user_enrichment_settings
    ALTER COLUMN video_clip_frames SET DEFAULT 4;

UPDATE user_enrichment_settings SET video_clip_frames = 4 WHERE video_clip_frames = 1;
