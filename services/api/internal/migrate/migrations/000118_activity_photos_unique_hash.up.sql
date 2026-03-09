-- Deduplicate existing photos with same (user_id, file_hash): keep the newest (highest id).
DELETE FROM activity_photos
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY user_id, file_hash ORDER BY id DESC) AS rn
        FROM activity_photos
        WHERE file_hash IS NOT NULL
    ) t WHERE rn > 1
);

-- Unique partial index to prevent duplicate photo imports (same file hash per user).
CREATE UNIQUE INDEX activity_photos_user_hash_unique
    ON activity_photos(user_id, file_hash)
    WHERE file_hash IS NOT NULL;
