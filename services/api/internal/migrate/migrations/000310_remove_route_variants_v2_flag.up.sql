-- Cosmetic cleanup: remove orphan server_settings row for the decommissioned
-- routing.route_variants_v2 feature flag. Settings store already tolerates
-- unknown keys (returns empty value), so this is DB hygiene, not a
-- functional requirement.
DELETE FROM server_settings WHERE key = 'routing.route_variants_v2';
