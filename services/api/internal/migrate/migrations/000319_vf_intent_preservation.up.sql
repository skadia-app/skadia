-- 000316_vf_intent_preservation.up.sql
--
-- Migrate user-customized routing_profiles that had max_via_ferrata >= 5 before
-- the AllowUnscaledViaFerrata safety-gate was introduced (Task 16a). Only users
-- who explicitly allowed K5/K6 (the hardest grades) get intent-preservation.
-- Users with max_via_ferrata < 5 (e.g. max_vf=3) wanted routes up to K3, NOT
-- necessarily unscaled via_ferrata — retroactive true would be a safety regression.
-- Builtin profiles (user_id IS NULL) and new users remain on the conservative
-- default (key absent = false).
UPDATE routing_profiles
SET config = jsonb_set(config, '{allow_unscaled_via_ferrata}', 'true'::jsonb, true),
    updated_at = now()
WHERE user_id IS NOT NULL
  AND (config->>'max_via_ferrata')::int >= 5;
