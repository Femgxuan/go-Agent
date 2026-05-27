-- migrations/002_add_fts.sql
-- 启用 pg_trgm 扩展（用于模糊匹配）
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 增加 tsvector 列
ALTER TABLE facts ADD COLUMN IF NOT EXISTS fts_vector tsvector;

-- 创建 GIN 索引
CREATE INDEX IF NOT EXISTS idx_facts_fts ON facts USING gin(fts_vector);

-- 创建触发器函数：自动更新 tsvector
CREATE OR REPLACE FUNCTION facts_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.key, '') || ' ' || COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 创建触发器
DROP TRIGGER IF EXISTS tsvector_update ON facts;
CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON facts
    FOR EACH ROW EXECUTE FUNCTION facts_fts_trigger();

-- 回填现有数据
UPDATE facts SET fts_vector = to_tsvector('simple', COALESCE(key, '') || ' ' || COALESCE(content, ''))
WHERE fts_vector IS NULL;
