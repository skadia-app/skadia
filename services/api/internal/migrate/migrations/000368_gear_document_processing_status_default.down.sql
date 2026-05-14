ALTER TABLE gear_documents
  DROP CONSTRAINT IF EXISTS gear_documents_processing_status_check;

ALTER TABLE gear_documents
  ALTER COLUMN processing_status SET DEFAULT 'pending';
