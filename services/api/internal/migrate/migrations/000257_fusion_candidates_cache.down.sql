DROP TRIGGER IF EXISTS trg_fusion_cache_upd ON activities;
DROP TRIGGER IF EXISTS trg_fusion_cache_ins_del ON activities;
DROP FUNCTION IF EXISTS invalidate_fusion_cache();
ALTER TABLE users DROP COLUMN IF EXISTS fusion_invalidation_seq;
ALTER TABLE users DROP COLUMN IF EXISTS fusion_computed_seq;
DROP TABLE IF EXISTS fusion_candidates;
