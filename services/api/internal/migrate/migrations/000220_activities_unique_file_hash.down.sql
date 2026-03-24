DROP INDEX IF EXISTS activities_unique_file_hash;
CREATE INDEX ON activities (file_hash) WHERE file_hash IS NOT NULL;
