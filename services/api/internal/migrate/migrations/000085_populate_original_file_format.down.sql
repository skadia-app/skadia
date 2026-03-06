-- Rollback: drop original_file_format column
ALTER TABLE activities DROP COLUMN IF EXISTS original_file_format;
