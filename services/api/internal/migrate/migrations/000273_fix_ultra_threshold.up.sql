-- Fix Ultra threshold: 42196m is too tight (GPS noise turns every marathon into ultra).
-- Change to 50000m (50km) which is the standard ultra-marathon entry distance.
-- Also adjust Marathon upper bound to match: ≥42000 AND <50000.
UPDATE automation_rules
SET branches = (
    SELECT jsonb_agg(
        CASE
            -- IF branch: change 42196 → 50000
            WHEN (branch->>'type') = 'if' THEN
                jsonb_set(
                    branch,
                    '{condition_groups,0,conditions}',
                    (SELECT jsonb_agg(
                        CASE WHEN (cond->>'field') = 'distance_m' AND (cond->>'op') = 'gte'
                             THEN jsonb_set(cond, '{value}', '50000'::jsonb)
                             ELSE cond
                        END
                    ) FROM jsonb_array_elements(branch->'condition_groups'->0->'conditions') AS cond)
                )
            ELSE branch
        END
    )
    FROM jsonb_array_elements(branches) AS branch
)
WHERE name = 'Laufdistanz' AND is_system = false AND branches IS NOT NULL;
