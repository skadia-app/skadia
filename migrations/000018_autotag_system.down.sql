-- Rollback: Autotag-System
ALTER TABLE tags DROP COLUMN IF EXISTS description;
ALTER TABLE tags DROP COLUMN IF EXISTS conditions;
ALTER TABLE tags DROP COLUMN IF EXISTS is_auto;
