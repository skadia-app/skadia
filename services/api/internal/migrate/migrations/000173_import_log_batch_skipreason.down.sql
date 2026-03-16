ALTER TABLE import_log
    DROP COLUMN IF EXISTS batch_id,
    DROP COLUMN IF EXISTS skip_reason;
