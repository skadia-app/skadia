ALTER TABLE user_enrichment_settings
    DROP COLUMN IF EXISTS ocr_invoice_prompt,
    DROP COLUMN IF EXISTS ocr_gear_prompt;
