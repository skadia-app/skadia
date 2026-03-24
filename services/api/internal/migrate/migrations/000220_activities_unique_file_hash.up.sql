-- Prevent duplicate activities from concurrent uploads of the same file.
-- The existing non-unique index is replaced by a unique partial index.
DROP INDEX IF EXISTS activities_file_hash_idx;
CREATE UNIQUE INDEX activities_unique_file_hash
    ON activities (user_id, file_hash)
    WHERE file_hash IS NOT NULL;
