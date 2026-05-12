ALTER TABLE user_enrichment_settings
    ADD COLUMN auto_title_display_policy TEXT NOT NULL DEFAULT 'source_unless_generic'
    CHECK (auto_title_display_policy IN ('source_unless_generic', 'source_first', 'auto_first'));
