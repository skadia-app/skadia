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
        SELECT jsonb_agg(
            CASE
                WHEN st.sub_type IS NULL OR st.sub_type = ''
                    THEN jsonb_build_object('sport', s.sport_type)
                ELSE jsonb_build_object('sport', s.sport_type, 'sub_type', st.sub_type)
            END
            ORDER BY s.ord
        )
        FROM unnest(allowed_sport_types) WITH ORDINALITY AS s(sport_type, ord)
        LEFT JOIN unnest(allowed_sub_types) WITH ORDINALITY AS st(sub_type, ord)
          ON st.ord = s.ord
    )
END
WHERE allowed_predicates IS NULL;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid,
    ADD CONSTRAINT segments_allowed_predicates_valid
        CHECK (allowed_predicates IS NULL OR fn_validate_predicates(allowed_predicates));
