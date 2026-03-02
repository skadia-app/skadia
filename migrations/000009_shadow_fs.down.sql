DROP TABLE IF EXISTS hotfolder_imports;
ALTER TABLE activities DROP COLUMN IF EXISTS local_file_path;
ALTER TABLE users DROP COLUMN IF EXISTS file_naming_schema;
