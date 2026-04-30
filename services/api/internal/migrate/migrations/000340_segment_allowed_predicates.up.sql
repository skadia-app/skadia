ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS allowed_predicates JSONB DEFAULT NULL;

WITH raw_hint_predicates AS (
    SELECT
        h.segment_id,
        jsonb_strip_nulls(jsonb_build_object(
            'sport', n.sport_type,
            'sub_type', NULLIF(n.sub_type, '')
        )) AS predicate
    FROM segments_legacy_sub_type_hints h
    CROSS JOIN LATERAL unnest(h.original_sport_types) WITH ORDINALITY AS raw(sport_type, ord)
    LEFT JOIN LATERAL unnest(COALESCE(h.original_sub_types, ARRAY[]::text[])) WITH ORDINALITY AS st(sub_type, ord)
      ON st.ord = raw.ord
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(raw.sport_type, COALESCE(st.sub_type, '')) AS n
),
hint_predicates AS (
    SELECT
        segment_id,
        jsonb_agg(predicate ORDER BY predicate::text) AS predicates
    FROM (
        SELECT DISTINCT segment_id, predicate
        FROM raw_hint_predicates
    ) deduped
    GROUP BY segment_id
)
UPDATE segments s
SET allowed_predicates = h.predicates
FROM hint_predicates h
WHERE s.id = h.segment_id
  AND s.allowed_predicates IS NULL;

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

DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT);
