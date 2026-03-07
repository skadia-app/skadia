-- Rollback: Remove geom_3857 column and performance indexes

DROP INDEX IF EXISTS idx_heatmap_lines_user_sport_date;
DROP INDEX IF EXISTS idx_heatmap_lines_user_date;
DROP INDEX IF EXISTS idx_heatmap_lines_sub_type;
DROP INDEX IF EXISTS idx_heatmap_lines_activity_date;
DROP INDEX IF EXISTS idx_heatmap_lines_geom_3857;

ALTER TABLE heatmap_lines DROP COLUMN IF EXISTS geom_3857;
