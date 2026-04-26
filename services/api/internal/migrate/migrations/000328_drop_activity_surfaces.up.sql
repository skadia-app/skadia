DO $$
BEGIN
    IF to_regclass('activity_surfaces') IS NOT NULL THEN
        INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m)
        SELECT
            activity_id,
            normalized_category AS category,
            enrichment_type AS type,
            -- Legacy rows may duplicate one bucket. Preserve their contribution, but
            -- clamp summed percentages to the replacement table's 0..100 constraint.
            LEAST(100, GREATEST(0, SUM(COALESCE(percentage, 0)))) AS percentage,
            SUM(COALESCE(length_m, 0)) AS length_m
        FROM (
            SELECT
                activity_id,
                COALESCE(NULLIF(category, ''), 'surface') AS normalized_category,
                CASE COALESCE(NULLIF(category, ''), 'surface')
                    WHEN 'highway' THEN COALESCE(NULLIF(path_type, ''), NULLIF(surface_type, ''), 'unknown')
                    ELSE COALESCE(NULLIF(surface_type, ''), NULLIF(path_type, ''), 'unknown')
                END AS enrichment_type,
                percentage,
                length_m
            FROM activity_surfaces
            WHERE COALESCE(NULLIF(category, ''), 'surface') IN ('surface', 'highway')
        ) legacy
        GROUP BY activity_id, normalized_category, enrichment_type
        ON CONFLICT (activity_id, category, type) DO NOTHING;
    END IF;
END $$;

DROP TABLE IF EXISTS activity_surfaces;
