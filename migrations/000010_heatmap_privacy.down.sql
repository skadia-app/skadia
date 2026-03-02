ALTER TABLE activities DROP COLUMN IF EXISTS data_visibility;
ALTER TABLE activities DROP COLUMN IF EXISTS hide_map;
DROP TABLE IF EXISTS activity_privacy_sections;
DROP TABLE IF EXISTS heatmap_lines;
DROP TABLE IF EXISTS user_privacy_zones;
