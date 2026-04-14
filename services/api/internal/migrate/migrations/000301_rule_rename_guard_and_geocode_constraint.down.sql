-- Reverse of 000301. The .up.sql is documentation + a RAISE NOTICE + a CHECK
-- constraint on reverse_geocode_cache.provider. Only the CHECK constraint is a
-- real schema change that needs to be reverted; the RAISE NOTICE has no
-- persistent effect.

ALTER TABLE reverse_geocode_cache
    DROP CONSTRAINT IF EXISTS reverse_geocode_cache_provider_check;
