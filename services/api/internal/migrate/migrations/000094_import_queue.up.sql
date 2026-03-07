CREATE TABLE import_queue (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    queue_type        TEXT NOT NULL CHECK (queue_type IN (
        'image_no_exif', 'image_ambiguous', 'pdf_untyped',
        'duplicate', 'import_error'
    )),
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved', 'dismissed')),
    original_filename TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    file_size_bytes   BIGINT,
    s3_bucket         TEXT,
    s3_key            TEXT,
    metadata          JSONB DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at       TIMESTAMPTZ
);

CREATE INDEX idx_import_queue_user_status ON import_queue(user_id, status);
CREATE INDEX idx_import_queue_created ON import_queue(created_at DESC);

COMMENT ON TABLE import_queue IS 'Queue for files needing manual review (images without EXIF, PDFs from hotfolder, duplicates, errors)';
COMMENT ON COLUMN import_queue.metadata IS 'EXIF data, candidate activity IDs, error details, duplicate info as JSON';
