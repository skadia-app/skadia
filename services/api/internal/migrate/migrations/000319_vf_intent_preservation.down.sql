-- 000319_vf_intent_preservation.down.sql
--
-- Remove the preserved-intent flag from custom profiles. Reverses the up
-- migration. Note: This loses information — users who manually set the flag
-- after the migration ran would also be reverted. Pragma: acceptable for a
-- down-migration (typically only used in dev/rollback, not prod).
UPDATE routing_profiles
SET config = config - 'allow_unscaled_via_ferrata',
    updated_at = now()
WHERE user_id IS NOT NULL
  AND (config->>'allow_unscaled_via_ferrata') = 'true';
