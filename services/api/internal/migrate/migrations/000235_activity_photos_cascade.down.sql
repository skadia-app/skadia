-- Revert: restore ON DELETE SET NULL.
ALTER TABLE activity_photos
    DROP CONSTRAINT IF EXISTS activity_photos_activity_id_fkey;

ALTER TABLE activity_photos
    ADD CONSTRAINT activity_photos_activity_id_fkey
        FOREIGN KEY (activity_id) REFERENCES activities(id) ON DELETE SET NULL;
