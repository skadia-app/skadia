-- products_found: count of products extracted by VLM pipeline
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS products_found INTEGER;

-- source_document_id on gear_components: link component back to source document
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS source_document_id UUID REFERENCES gear_documents(id) ON DELETE SET NULL;
