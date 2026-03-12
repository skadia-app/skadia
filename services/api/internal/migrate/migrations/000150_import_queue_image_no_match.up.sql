ALTER TABLE import_queue DROP CONSTRAINT IF EXISTS import_queue_queue_type_check;
ALTER TABLE import_queue ADD CONSTRAINT import_queue_queue_type_check
  CHECK (queue_type IN ('image_no_exif', 'image_ambiguous', 'image_no_match', 'pdf_untyped', 'duplicate', 'import_error'));
