CREATE TABLE gear_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    gear_id         UUID REFERENCES gear(id) ON DELETE CASCADE,
    component_id    UUID REFERENCES gear_components(id) ON DELETE CASCADE,
    service_event_id UUID REFERENCES gear_service_events(id) ON DELETE CASCADE,
    doc_type        TEXT NOT NULL DEFAULT 'photo'
                    CHECK (doc_type IN ('receipt','invoice','warranty','manual','photo','other')),
    title           TEXT,
    description     TEXT,
    original_filename TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    file_hash       TEXT,
    s3_bucket       TEXT NOT NULL,
    s3_key          TEXT NOT NULL,
    document_date   DATE,
    document_amount NUMERIC(10,2),
    document_currency TEXT DEFAULT 'EUR',
    extracted_markdown TEXT,
    extracted_data  JSONB,
    processing_status TEXT DEFAULT 'pending',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT gear_documents_target_check CHECK (
        gear_id IS NOT NULL OR component_id IS NOT NULL OR service_event_id IS NOT NULL
    )
);
CREATE INDEX ON gear_documents (gear_id) WHERE gear_id IS NOT NULL;
CREATE INDEX ON gear_documents (component_id) WHERE component_id IS NOT NULL;
CREATE INDEX ON gear_documents (user_id, created_at DESC);
