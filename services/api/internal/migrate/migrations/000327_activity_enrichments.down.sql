ALTER TABLE users DROP COLUMN IF EXISTS surface_extension_acknowledged_at;

ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS enable_difficulty_scales;

DROP TABLE IF EXISTS activity_enrichments;
