-- Preflight for migration 000339.
-- Returns automation rules whose OR groups contain legacy sport predicates that
-- cannot be tuple-expanded without changing boolean semantics.

WITH legacy_sports(sport_type) AS (
    VALUES
        ('mountain_biking'), ('gravel_cycling'), ('indoor_cycling'),
        ('trail_running'), ('treadmill_running'),
        ('lap_swimming'), ('open_water_swimming'),
        ('bike'), ('biking'), ('ride'), ('road_cycling'), ('roadcycling'),
        ('e_mountain_biking'), ('e_biking'), ('e_bike')
),
top_level_groups AS (
    SELECT
        r.id,
        r.name,
        'condition_groups'::text AS surface,
        group_item.value AS condition_group
    FROM automation_rules r
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(r.condition_groups, '[]'::jsonb)) AS group_item(value)
    WHERE jsonb_typeof(COALESCE(r.condition_groups, '[]'::jsonb)) = 'array'
),
branch_groups AS (
    SELECT
        r.id,
        r.name,
        'branches[].condition_groups'::text AS surface,
        group_item.value AS condition_group
    FROM automation_rules r
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(r.branches, '[]'::jsonb)) AS branch_item(value)
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(branch_item.value->'condition_groups', '[]'::jsonb)) AS group_item(value)
    WHERE jsonb_typeof(COALESCE(r.branches, '[]'::jsonb)) = 'array'
      AND jsonb_typeof(COALESCE(branch_item.value->'condition_groups', '[]'::jsonb)) = 'array'
),
all_groups AS (
    SELECT * FROM top_level_groups
    UNION ALL
    SELECT * FROM branch_groups
),
or_conditions AS (
    SELECT
        g.id,
        g.name,
        g.surface,
        condition_item.value AS condition
    FROM all_groups g
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(g.condition_group->'conditions', '[]'::jsonb)) AS condition_item(value)
    WHERE lower(COALESCE(g.condition_group->>'logic', 'and')) = 'or'
      AND jsonb_typeof(COALESCE(g.condition_group->'conditions', '[]'::jsonb)) = 'array'
)
SELECT
    id,
    name,
    surface,
    condition,
    'OR group contains a legacy sport_type predicate that must be resolved before 000339' AS reason
FROM or_conditions
WHERE condition->>'field' = 'sport_type'
  AND condition->>'value' IN (SELECT sport_type FROM legacy_sports)
ORDER BY name, id, surface;
