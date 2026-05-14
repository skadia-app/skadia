ALTER TABLE gear_documents
  ALTER COLUMN processing_status SET DEFAULT 'idle';

UPDATE gear_documents
SET processing_status = 'idle'
WHERE processing_status IS NULL;

UPDATE gear_documents
SET processing_status = 'failed'
WHERE processing_status NOT IN ('idle', 'pending', 'processed', 'failed');

UPDATE gear_documents
SET processing_status = 'idle'
WHERE processing_status = 'pending'
  AND content_type LIKE 'image/%';

ALTER TABLE gear_documents
  ADD CONSTRAINT gear_documents_processing_status_check
  CHECK (processing_status IN ('idle', 'pending', 'processed', 'failed'));
