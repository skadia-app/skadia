-- Migration 000136: CLIP embedding für semantische Foto-Suche (SigLIP2, 1152 dims)

-- Embedding-Spalte
ALTER TABLE activity_photos
    ADD COLUMN IF NOT EXISTS clip_embedding vector(1152);

-- HNSW Index (pgvector, immer verfuegbar)
-- Kein CONCURRENTLY da go-migrate in Transaktionen laeuft
CREATE INDEX IF NOT EXISTS idx_activity_photos_clip_embedding
    ON activity_photos USING hnsw (clip_embedding vector_cosine_ops);

-- User-Setting: CLIP-Embedding pro User deaktivierbar
ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS enable_photo_clip_embedding BOOLEAN NOT NULL DEFAULT true;
