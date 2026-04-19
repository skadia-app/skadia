-- Restore the redundant function + rebind the trigger. Purely aesthetic rollback.
-- NOTE: the function body below was identical to invalidate_fusion_cache at the time
-- of mig 000307. If invalidate_fusion_cache has since drifted, this rollback will no
-- longer perfectly reverse the consolidation — intentional, documented in ADR.
-- See ADR: docs/internal/2026-04-16-adr-fusion-cache-trigger-consolidation.md

CREATE OR REPLACE FUNCTION fn_bump_fusion_cache_seq() RETURNS trigger AS $$
DECLARE
    affected_user UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_user := OLD.user_id;
    ELSE
        affected_user := NEW.user_id;
    END IF;
    UPDATE users SET fusion_invalidation_seq = fusion_invalidation_seq + 1
    WHERE id = affected_user;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fusion_cache_memberships ON activity_fusion_memberships;
CREATE TRIGGER trg_fusion_cache_memberships
    AFTER INSERT OR DELETE OR UPDATE ON activity_fusion_memberships
    FOR EACH ROW
    EXECUTE FUNCTION fn_bump_fusion_cache_seq();
