-- Rollback: clear original_file_format values that were auto-populated during backfill
-- (We only clear them if original_file_format was NULL before, which is all of them in this context)
UPDATE activities
SET original_file_format = NULL
WHERE original_file_format IS NOT NULL;
