ALTER TABLE import_log
    ADD COLUMN batch_id    UUID,
    ADD COLUMN skip_reason TEXT;

CREATE INDEX ON import_log (user_id, batch_id) WHERE batch_id IS NOT NULL;
