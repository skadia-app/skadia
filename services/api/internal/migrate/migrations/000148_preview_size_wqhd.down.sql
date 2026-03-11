UPDATE user_enrichment_settings SET photo_preview_size = '2048' WHERE photo_preview_size = 'wqhd';
ALTER TABLE user_enrichment_settings ALTER COLUMN photo_preview_size SET DEFAULT '2048';
