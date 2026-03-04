DROP TABLE IF EXISTS activity_surfaces;
ALTER TABLE gear DROP COLUMN IF EXISTS review_status;
ALTER TABLE gear_components DROP COLUMN IF EXISTS review_status;
ALTER TABLE gear DROP COLUMN IF EXISTS weight_g;
ALTER TABLE gear DROP COLUMN IF EXISTS source_document_id;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS extracted_text;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS processing_status;
