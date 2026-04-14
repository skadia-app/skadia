-- Follow-up to Finding #26 (review of migrations 000298/000299) and Finding #28
-- (reverse_geocode_cache provider constraint).
--
-- Background #26:
--   Migrations 000298 and 000299 did a blind
--       UPDATE automation_rules SET name = '<new>' WHERE name = '<old>';
--   WITHOUT filtering on is_system = true. That means if a user manually created
--   a non-system automation rule literally named "Intervall-Training" or
--   "Intervalltraining", the bulk rename in those migrations silently overwrote
--   the user's own rule name. Both migrations are already applied in production,
--   so the damage (if any) cannot be undone retroactively: we cannot tell
--   which of today's rows were user-owned rules that got overwritten vs. rules
--   that genuinely carried the system name.
--
-- Strategy (Option A — conservative):
--   Do NOT touch any existing data. Retroactively "guessing" which user rules
--   were corrupted (e.g. by last_evaluated_at IS NULL heuristics) would risk
--   overwriting legitimate manual renames and cause more harm than good.
--   Instead:
--     * This migration is a documentation-only / guardrail change.
--     * The code side gets a helper RenameSystemRule() that *always* filters on
--       is_system=true, so future bulk renames can't repeat the mistake.
--     * A RAISE NOTICE below surfaces if the problematic name pattern is still
--       present, purely for operator awareness during `migrate up`.
--
-- Background #28:
--   reverse_geocode_cache.provider (added in 000300) is a free-text column and
--   the Go code uses the literal strings "nominatim" and "photon". A typo in
--   the Go layer ("Nominatim", "photon ", …) would silently produce cache rows
--   that never match on lookup, because Lookup() passes the same typo-string
--   back in. Lock it down with a CHECK constraint so any such bug becomes a
--   hard failure instead of a silent cache-miss.

DO $$
DECLARE
    rule_count INT;
BEGIN
    SELECT COUNT(*) INTO rule_count
      FROM automation_rules
     WHERE is_system = false
       AND name IN ('Intervalltraining', 'Intervall-Training');
    IF rule_count > 0 THEN
        RAISE NOTICE
            'rule-rename-guard: % non-system automation_rules rows currently use '
            'one of the interval-tag names. Migrations 000298/000299 did a blind '
            'UPDATE without an is_system filter; if any of these are user-owned '
            'renames that were clobbered, there is no way to restore them. '
            'No action required if no user-owned renames existed (the count will '
            'also be > 0 when users legitimately named their own rules after the '
            'system tags). If the count is > 0 and you suspect a clobbered rename, '
            'review those rules in the admin UI and rename them manually — future '
            'bulk renames go through RenameSystemRule(), which filters on '
            'is_system=true and cannot clobber user rules again.', rule_count;
    END IF;
END $$;

-- Finding #28: Lock reverse_geocode_cache.provider to the two legal values so
-- typos in the Go layer fail fast instead of producing unreachable cache rows.
-- If for any reason a row with an illegal provider exists (shouldn't happen —
-- table was created empty in 000300 and the Go layer only writes "nominatim" /
-- "photon"), drop it before adding the constraint.
DELETE FROM reverse_geocode_cache
 WHERE provider NOT IN ('nominatim', 'photon');

ALTER TABLE reverse_geocode_cache
    ADD CONSTRAINT reverse_geocode_cache_provider_check
    CHECK (provider IN ('nominatim', 'photon'));
