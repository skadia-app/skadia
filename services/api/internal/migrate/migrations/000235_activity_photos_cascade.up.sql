-- M38: activity_photos.activity_id ON DELETE SET NULL → ON DELETE CASCADE.
-- When an activity is deleted, its photos should be deleted too (not orphaned
-- with activity_id=NULL). The Go handler cleans up S3/ShadowFS objects first,
-- so CASCADE only affects the DB rows.
ALTER TABLE activity_photos
    DROP CONSTRAINT IF EXISTS activity_photos_activity_id_fkey;

ALTER TABLE activity_photos
    ADD CONSTRAINT activity_photos_activity_id_fkey
        FOREIGN KEY (activity_id) REFERENCES activities(id) ON DELETE CASCADE;
