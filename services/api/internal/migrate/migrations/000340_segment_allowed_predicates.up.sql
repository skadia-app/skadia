ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS allowed_predicates JSONB DEFAULT NULL;

UPDATE segments
SET allowed_predicates = CASE
    WHEN allowed_sport_types IS NULL THEN NULL
    WHEN cardinality(allowed_sport_types) = 0 THEN '[]'::jsonb
    WHEN allowed_sub_types IS NULL OR cardinality(allowed_sub_types) = 0 THEN (
        SELECT jsonb_agg(jsonb_build_object('sport', sport_type) ORDER BY sport_type)
        FROM unnest(allowed_sport_types) AS s(sport_type)
    )
    ELSE (
        SELECT jsonb_agg(jsonb_build_object('sport', sport_type, 'sub_type', sub_type) ORDER BY sport_type, sub_type)
        FROM unnest(allowed_sport_types) AS s(sport_type)
        CROSS JOIN unnest(allowed_sub_types) AS st(sub_type)
    )
END
WHERE allowed_predicates IS NULL;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid,
    ADD CONSTRAINT segments_allowed_predicates_valid
        CHECK (allowed_predicates IS NULL OR fn_validate_predicates(allowed_predicates));
