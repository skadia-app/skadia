ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS ocr_invoice_prompt TEXT,
    ADD COLUMN IF NOT EXISTS ocr_gear_prompt TEXT;

COMMENT ON COLUMN user_enrichment_settings.ocr_invoice_prompt IS 'Custom Prompt für Rechnungs-OCR. NULL = Default.';
COMMENT ON COLUMN user_enrichment_settings.ocr_gear_prompt IS 'Custom Prompt für Gear-Extraktion. NULL = Default.';
