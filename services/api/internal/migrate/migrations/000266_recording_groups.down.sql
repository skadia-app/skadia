DROP TRIGGER IF EXISTS trg_group_primary_sync ON recording_groups;
DROP TRIGGER IF EXISTS trg_group_members_sync ON recording_group_members;
DROP FUNCTION IF EXISTS fn_sync_group_secondary();
DROP INDEX IF EXISTS idx_activities_group_secondary;
ALTER TABLE activities DROP COLUMN IF EXISTS is_group_secondary;
DROP TABLE IF EXISTS recording_group_members;
DROP TABLE IF EXISTS recording_groups;
