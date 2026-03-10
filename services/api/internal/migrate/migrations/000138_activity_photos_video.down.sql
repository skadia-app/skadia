ALTER TABLE activity_photos
    DROP COLUMN IF EXISTS is_video,
    DROP COLUMN IF EXISTS duration_s,
    DROP COLUMN IF EXISTS video_width,
    DROP COLUMN IF EXISTS video_height;
