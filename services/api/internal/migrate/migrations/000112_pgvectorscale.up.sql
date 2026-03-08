-- Activate pgvector extension (required for VECTOR type and embeddings)
CREATE EXTENSION IF NOT EXISTS vector;

-- Activate pgvectorscale (provides DiskANN index, depends on pgvector)
-- Falls pgvectorscale nicht im Image verfügbar: nur pgvector reicht für HNSW-Index
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vectorscale CASCADE;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvectorscale not available — using pgvector HNSW instead of DiskANN: %', SQLERRM;
END
$$;
