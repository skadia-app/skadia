-- Add original_file_format column to store file format (fit, gpx, tcx)
-- This fixes export/download which extract from S3 key instead of filename extension
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_format TEXT DEFAULT NULL;

-- Backfill original_file_format for existing activities based on S3 key extension
-- S3 keys follow pattern: {userID}/{hash}.{format}
-- Extract format from the rightmost part after the last dot

UPDATE activities
SET original_file_format = LOWER(
  COALESCE(
    -- Try to extract from original_file_key using split_part on the last dot
    NULLIF(SPLIT_PART(original_file_key, '.', ARRAY_LENGTH(STRING_TO_ARRAY(original_file_key, '.'), 1)), ''),
    -- Fallback: extract from original_filename
    NULLIF(SPLIT_PART(original_filename, '.', ARRAY_LENGTH(STRING_TO_ARRAY(original_filename, '.'), 1)), ''),
    -- Default to 'fit'
    'fit'
  )
)
WHERE original_file_format IS NULL;
