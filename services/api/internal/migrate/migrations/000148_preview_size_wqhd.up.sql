-- Rename the '2048' preview size value to 'wqhd' (2560px, actual WQHD resolution).
UPDATE user_enrichment_settings SET photo_preview_size = 'wqhd' WHERE photo_preview_size = '2048';
ALTER TABLE user_enrichment_settings ALTER COLUMN photo_preview_size SET DEFAULT 'wqhd';
