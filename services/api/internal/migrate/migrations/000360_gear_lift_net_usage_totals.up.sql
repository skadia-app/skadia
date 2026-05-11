-- Recompute stored gear usage totals with the same lift-subtracted net
-- semantics used by runtime gear refreshes. Migration 000355 introduced
-- activity_gear_assignments and backfilled gear totals from gross activity
-- values; this normalizes existing rows after lift descent/distance/ascent
-- subtraction became part of gear aggregation.

WITH usage AS (
    SELECT
        g.id AS gear_id,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(
            aga.distance_m_override,
            GREATEST(0,
                COALESCE(a.distance_m, 0) -
                CASE WHEN a.exclude_lift_subtraction THEN 0
                     ELSE COALESCE(a.lift_distance_m, 0) END),
            0) END), 0) AS total_distance_m,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(aga.moving_time_s_override, a.moving_time_s, 0) END), 0) AS total_moving_time_s,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(a.elapsed_time_s, 0) END), 0) AS total_time_s,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_gain_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_ascent_m, 0) END) END), 0) AS total_elevation_gain_m,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_loss_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_descent_m, 0) END) END), 0) AS total_elevation_loss_m,
        COUNT(a.id) AS total_activities
    FROM gear g
    LEFT JOIN activity_gear_assignments aga ON aga.gear_id = g.id
    LEFT JOIN activities a ON a.id = aga.activity_id
        AND NOT a.is_group_secondary
    GROUP BY g.id
)
UPDATE gear g
SET total_distance_m = usage.total_distance_m,
    total_moving_time_s = usage.total_moving_time_s,
    total_time_s = usage.total_time_s,
    total_elevation_gain_m = usage.total_elevation_gain_m,
    total_elevation_loss_m = usage.total_elevation_loss_m,
    total_activities = usage.total_activities
FROM usage
WHERE g.id = usage.gear_id;

WITH usage AS (
    SELECT
        gc.id AS component_id,
        ca.id AS assignment_id,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(
            aga.distance_m_override,
            GREATEST(0,
                COALESCE(a.distance_m, 0) -
                CASE WHEN a.exclude_lift_subtraction THEN 0
                     ELSE COALESCE(a.lift_distance_m, 0) END),
            0) END), 0) AS distance_m,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(aga.moving_time_s_override, a.moving_time_s, 0) END), 0) AS moving_time_s,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE COALESCE(a.elapsed_time_s, 0) END), 0) AS time_s,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_gain_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_ascent_m, 0) END) END), 0) AS elevation_gain_m,
        COALESCE(SUM(CASE WHEN a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_loss_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_descent_m, 0) END) END), 0) AS elevation_loss_m,
        COUNT(a.id) AS activities_count
    FROM gear_components gc
    LEFT JOIN component_assignments ca ON ca.component_id = gc.id AND ca.removed_at IS NULL
    LEFT JOIN activity_gear_assignments aga ON aga.gear_id = gc.gear_id
    LEFT JOIN activities a ON a.id = aga.activity_id
        AND NOT a.is_group_secondary
        AND a.started_at >= COALESCE(ca.installed_at, gc.installed_at, '-infinity'::timestamptz)
        AND a.started_at < COALESCE(ca.removed_at, gc.removed_at, 'infinity'::timestamptz)
    GROUP BY gc.id, ca.id
),
update_component AS (
    UPDATE gear_components gc
    SET total_distance_m = usage.distance_m,
        total_moving_time_s = usage.moving_time_s,
        total_time_s = usage.time_s,
        total_elevation_gain_m = usage.elevation_gain_m,
        total_elevation_loss_m = usage.elevation_loss_m,
        total_activities = usage.activities_count
    FROM usage
    WHERE gc.id = usage.component_id
    RETURNING usage.*
)
UPDATE component_assignments ca
SET distance_m = update_component.distance_m,
    moving_time_s = update_component.moving_time_s,
    time_s = update_component.time_s,
    elevation_gain_m = update_component.elevation_gain_m,
    elevation_loss_m = update_component.elevation_loss_m,
    activities_count = update_component.activities_count
FROM update_component
WHERE ca.id = update_component.assignment_id;

WITH usage AS (
    SELECT
        cm.id AS member_id,
        COALESCE(SUM(CASE WHEN NOT (cm.is_active = true OR cm.last_swapped_at IS NOT NULL) OR a.id IS NULL THEN 0 ELSE COALESCE(
            aga.distance_m_override,
            GREATEST(0,
                COALESCE(a.distance_m, 0) -
                CASE WHEN a.exclude_lift_subtraction THEN 0
                     ELSE COALESCE(a.lift_distance_m, 0) END),
            0) END), 0) AS total_distance_m,
        COALESCE(SUM(CASE WHEN NOT (cm.is_active = true OR cm.last_swapped_at IS NOT NULL) OR a.id IS NULL THEN 0 ELSE COALESCE(aga.moving_time_s_override, a.moving_time_s, 0) END), 0) AS total_moving_time_s,
        COALESCE(SUM(CASE WHEN NOT (cm.is_active = true OR cm.last_swapped_at IS NOT NULL) OR a.id IS NULL THEN 0 ELSE COALESCE(a.elapsed_time_s, 0) END), 0) AS total_time_s,
        COALESCE(SUM(CASE WHEN NOT (cm.is_active = true OR cm.last_swapped_at IS NOT NULL) OR a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_gain_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_ascent_m, 0) END) END), 0) AS total_elevation_gain_m,
        COALESCE(SUM(CASE WHEN NOT (cm.is_active = true OR cm.last_swapped_at IS NOT NULL) OR a.id IS NULL THEN 0 ELSE GREATEST(0,
            COALESCE(a.elevation_loss_m, 0) -
            CASE WHEN a.exclude_lift_subtraction THEN 0
                 ELSE COALESCE(a.lift_descent_m, 0) END) END), 0) AS total_elevation_loss_m,
        COUNT(a.id) FILTER (WHERE cm.is_active = true OR cm.last_swapped_at IS NOT NULL) AS total_activities
    FROM chain_rotation_members cm
    JOIN chain_rotation_groups crg ON crg.id = cm.group_id
    JOIN gear_components gc ON gc.id = cm.component_id
    LEFT JOIN activity_gear_assignments aga ON aga.gear_id = crg.gear_id
    LEFT JOIN activities a ON a.id = aga.activity_id
        AND NOT a.is_group_secondary
        AND a.started_at >= COALESCE(cm.last_swapped_at, crg.created_at)
        AND a.started_at >= COALESCE(gc.installed_at, '-infinity'::timestamptz)
        AND a.started_at < COALESCE(gc.removed_at, 'infinity'::timestamptz)
    GROUP BY cm.id
)
UPDATE chain_rotation_members cm
SET total_distance_m = usage.total_distance_m,
    total_moving_time_s = usage.total_moving_time_s,
    total_time_s = usage.total_time_s,
    total_elevation_gain_m = usage.total_elevation_gain_m,
    total_elevation_loss_m = usage.total_elevation_loss_m,
    total_activities = usage.total_activities
FROM usage
WHERE cm.id = usage.member_id;
