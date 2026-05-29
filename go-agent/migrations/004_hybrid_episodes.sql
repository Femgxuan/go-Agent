-- migrations/004_hybrid_episodes.sql
-- Hybrid retrieval: embedding + HNSW + tsvector for episodes

-- 1. Ensure pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Add embedding column to episodes
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- 3. Create HNSW index for vector search
CREATE INDEX IF NOT EXISTS idx_episodes_embedding_hnsw
  ON episodes USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);

-- 4. Ensure tsvector column exists
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS fts_vector tsvector;

-- 5. Create GIN index for FTS
CREATE INDEX IF NOT EXISTS idx_episodes_fts_new ON episodes USING gin(fts_vector);

-- 6. Trigger function: auto-update tsvector on insert/update
CREATE OR REPLACE FUNCTION episodes_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tsvector_update ON episodes;
CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON episodes
    FOR EACH ROW EXECUTE FUNCTION episodes_fts_trigger();

-- 7. Backfill existing rows
UPDATE episodes SET fts_vector = to_tsvector('simple', COALESCE(content, ''))
WHERE fts_vector IS NULL;
