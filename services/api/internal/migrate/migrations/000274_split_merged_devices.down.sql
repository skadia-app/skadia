-- 000274_split_merged_devices.down.sql
-- Cannot automatically reverse the device split (original merged records were deleted).
-- A manual rollback would require re-merging devices by manufacturer and relinking activities.
SELECT 1;
