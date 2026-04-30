ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS allowed_sport_types TEXT[],
    ADD COLUMN IF NOT EXISTS allowed_sub_types TEXT[];

UPDATE segments
SET
    allowed_sport_types = derived.sport_types,
    allowed_sub_types = derived.sub_types
FROM (
    SELECT
        s.id,
        CASE
            WHEN s.allowed_predicates IS NULL THEN NULL::text[]
            ELSE ARRAY(
                SELECT DISTINCT pred->>'sport'
                FROM jsonb_array_elements(s.allowed_predicates) pred
                WHERE pred ? 'sport'
                ORDER BY pred->>'sport'
            )
        END AS sport_types,
        CASE
            WHEN s.allowed_predicates IS NULL THEN NULL::text[]
            ELSE ARRAY(
                SELECT DISTINCT pred->>'sub_type'
                FROM jsonb_array_elements(s.allowed_predicates) pred
                WHERE pred ? 'sub_type' AND pred->>'sub_type' <> ''
                ORDER BY pred->>'sub_type'
            )
        END AS sub_types
    FROM segments s
) derived
WHERE segments.id = derived.id
  AND segments.allowed_predicates IS NOT NULL;

ALTER TABLE planned_routes
    DROP COLUMN IF EXISTS is_electric,
    DROP COLUMN IF EXISTS is_indoor,
    DROP COLUMN IF EXISTS is_virtual,
    DROP COLUMN IF EXISTS is_commute;
