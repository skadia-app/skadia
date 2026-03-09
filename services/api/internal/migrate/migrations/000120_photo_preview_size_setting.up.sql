-- User preference for preview thumbnail size: '2048' (default), '4k', or 'original'.
ALTER TABLE user_enrichment_settings
    ADD COLUMN photo_preview_size TEXT NOT NULL DEFAULT '2048';
