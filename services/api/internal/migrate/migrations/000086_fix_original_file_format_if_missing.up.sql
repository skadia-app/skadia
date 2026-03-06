-- Ensure original_file_format column exists and is properly populated
-- This is a safety check in case 000085 didn't fully complete or had issues

-- First, ensure the column exists (should already exist from 000003, but double-check)
-- Using IF NOT EXISTS to make this idempotent
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_format TEXT DEFAULT NULL;

-- Backfill any NULL values that weren't filled by migration 000085
-- Extract from S3 key first (most reliable: {userID}/{hash}.{format})
-- Then from original_filename as fallback
UPDATE activities
SET original_file_format = LOWER(
  COALESCE(
    -- Try S3 key: extract rightmost part after last dot
    NULLIF(SPLIT_PART(original_file_key, '.', ARRAY_LENGTH(STRING_TO_ARRAY(original_file_key, '.'), 1)), ''),
    -- Fallback: filename extension
    NULLIF(SPLIT_PART(original_filename, '.', ARRAY_LENGTH(STRING_TO_ARRAY(original_filename, '.'), 1)), ''),
    -- Default fallback
    'fit'
  )
)
WHERE original_file_format IS NULL OR original_file_format = '';
