-- activity_surfaces: ML Stage 2 surface detection results
CREATE TABLE IF NOT EXISTS activity_surfaces (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    surface_type    TEXT NOT NULL,
    path_type       TEXT NOT NULL DEFAULT 'unknown',
    length_m        DOUBLE PRECISION,
    percentage      DOUBLE PRECISION
);
CREATE INDEX IF NOT EXISTS idx_activity_surfaces_activity ON activity_surfaces (activity_id);

-- Review-Status for automatically detected Gear data (VLM-Pipeline)
ALTER TABLE gear ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'none';
-- 'none' | 'pending_review' | 'confirmed' | 'rejected'
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'none';

-- weight_g on gear (if not already present)
ALTER TABLE gear ADD COLUMN IF NOT EXISTS weight_g INTEGER;

-- source_document_id: link gear back to the document that created it
ALTER TABLE gear ADD COLUMN IF NOT EXISTS source_document_id UUID REFERENCES gear_documents(id) ON DELETE SET NULL;

-- extracted_text + processing_status on gear_documents for pdfcpu/VLM pipeline
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS extracted_text TEXT;
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS processing_status TEXT NOT NULL DEFAULT 'idle';
-- 'idle' | 'processing' | 'done' | 'error'
