-- Fix: ELSE-IF branches for Laufdistanz were missing sport_type=running condition.
-- Without it, a 42km bike ride would be tagged "Marathon".
-- Add sport_type=running to all else_if branches.
UPDATE automation_rules
SET branches = (
    SELECT jsonb_agg(
        CASE
            WHEN (branch->>'type') IN ('else_if') AND NOT EXISTS (
                SELECT 1 FROM jsonb_array_elements(branch->'condition_groups') AS cg,
                             jsonb_array_elements(cg->'conditions') AS cond
                WHERE cond->>'field' = 'sport_type'
            )
            THEN jsonb_set(
                branch,
                '{condition_groups,0,conditions}',
                (branch->'condition_groups'->0->'conditions') || '{"field":"sport_type","op":"eq","value":"running"}'::jsonb
            )
            ELSE branch
        END
    )
    FROM jsonb_array_elements(branches) AS branch
)
WHERE name = 'Laufdistanz' AND is_system = false AND branches IS NOT NULL;
