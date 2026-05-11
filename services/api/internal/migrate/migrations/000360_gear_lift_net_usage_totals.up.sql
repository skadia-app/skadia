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
