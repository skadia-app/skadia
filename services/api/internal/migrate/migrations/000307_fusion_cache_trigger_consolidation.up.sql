-- Phase 5 follow-up — consolidate cache-invalidation functions.
-- Pre-migration: two functions with identical bodies existed (invalidate_fusion_cache
-- and fn_bump_fusion_cache_seq); each fusion mutation bumped fusion_invalidation_seq
-- twice (memberships INSERT + activities UPDATE). This change makes the membership
-- trigger delegate to the canonical invalidate_fusion_cache function. Single source
-- of truth for the bump semantics.
BEGIN;

-- Drop the memberships trigger so we can change its function reference.
DROP TRIGGER IF EXISTS trg_fusion_cache_memberships ON activity_fusion_memberships;

-- Recreate the memberships trigger using the canonical invalidate_fusion_cache.
-- Preserves the same fire-on-INSERT/UPDATE/DELETE semantics from Mig 000302.
CREATE TRIGGER trg_fusion_cache_memberships
    AFTER INSERT OR UPDATE OR DELETE ON activity_fusion_memberships
    FOR EACH ROW
    EXECUTE FUNCTION invalidate_fusion_cache();

-- Drop the duplicate function.
DROP FUNCTION IF EXISTS fn_bump_fusion_cache_seq();

COMMIT;
