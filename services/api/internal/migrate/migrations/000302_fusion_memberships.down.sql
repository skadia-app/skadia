-- 000302_fusion_memberships.down.sql
BEGIN;
DROP TRIGGER IF EXISTS trg_fusion_cache_memberships ON activity_fusion_memberships;
DROP FUNCTION IF EXISTS fn_bump_fusion_cache_seq();
DROP TABLE IF EXISTS activity_fusion_memberships;
COMMIT;
