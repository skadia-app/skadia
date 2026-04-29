-- WARNING: This down migration is destructive for post-cleanup sub_types.
-- It exists for test/dev rollback only and is not a production rollback plan.

ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_sport_type_canonical;
ALTER TABLE routing_profiles DROP CONSTRAINT IF EXISTS routing_profiles_predicates_valid;
ALTER TABLE routing_profiles DROP CONSTRAINT IF EXISTS routing_profiles_modifier_filters_valid;

UPDATE activities SET sport_type = 'mountain_biking', sub_type = NULL
WHERE sport_type = 'cycling' AND sub_type = 'mountain';
UPDATE activities SET sport_type = 'gravel_cycling', sub_type = NULL
WHERE sport_type = 'cycling' AND sub_type = 'gravel_cycling';
UPDATE activities SET sport_type = 'indoor_cycling', sub_type = NULL
WHERE sport_type = 'cycling' AND sub_type = 'indoor_cycling';
UPDATE activities SET sport_type = 'trail_running', sub_type = NULL
WHERE sport_type = 'running' AND sub_type = 'trail';
UPDATE activities SET sport_type = 'treadmill_running', sub_type = NULL
WHERE sport_type = 'running' AND sub_type = 'indoor_running';
UPDATE activities SET sport_type = 'lap_swimming', sub_type = NULL
WHERE sport_type = 'swimming' AND sub_type = 'lap_swimming';
UPDATE activities SET sport_type = 'open_water_swimming', sub_type = NULL
WHERE sport_type = 'swimming' AND sub_type = 'open_water';

UPDATE activities SET parsed_sport_type = 'mountain_biking', parsed_sub_type = NULL
WHERE parsed_sport_type = 'cycling' AND parsed_sub_type = 'mountain';
UPDATE activities SET parsed_sport_type = 'gravel_cycling', parsed_sub_type = NULL
WHERE parsed_sport_type = 'cycling' AND parsed_sub_type = 'gravel_cycling';
UPDATE activities SET parsed_sport_type = 'indoor_cycling', parsed_sub_type = NULL
WHERE parsed_sport_type = 'cycling' AND parsed_sub_type = 'indoor_cycling';
UPDATE activities SET parsed_sport_type = 'trail_running', parsed_sub_type = NULL
WHERE parsed_sport_type = 'running' AND parsed_sub_type = 'trail';
UPDATE activities SET parsed_sport_type = 'treadmill_running', parsed_sub_type = NULL
WHERE parsed_sport_type = 'running' AND parsed_sub_type = 'indoor_running';
UPDATE activities SET parsed_sport_type = 'lap_swimming', parsed_sub_type = NULL
WHERE parsed_sport_type = 'swimming' AND parsed_sub_type = 'lap_swimming';
UPDATE activities SET parsed_sport_type = 'open_water_swimming', parsed_sub_type = NULL
WHERE parsed_sport_type = 'swimming' AND parsed_sub_type = 'open_water';

UPDATE heatmap_lines SET sport_type = 'mountain_biking', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'mountain';
UPDATE heatmap_lines SET sport_type = 'gravel_cycling', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'gravel_cycling';
UPDATE heatmap_lines SET sport_type = 'indoor_cycling', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'indoor_cycling';
UPDATE heatmap_lines SET sport_type = 'trail_running', sub_type = NULL WHERE sport_type = 'running' AND sub_type = 'trail';
UPDATE heatmap_lines SET sport_type = 'treadmill_running', sub_type = NULL WHERE sport_type = 'running' AND sub_type = 'indoor_running';
UPDATE heatmap_lines SET sport_type = 'lap_swimming', sub_type = NULL WHERE sport_type = 'swimming' AND sub_type = 'lap_swimming';
UPDATE heatmap_lines SET sport_type = 'open_water_swimming', sub_type = NULL WHERE sport_type = 'swimming' AND sub_type = 'open_water';

UPDATE heatmap_points SET sport_type = 'mountain_biking', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'mountain';
UPDATE heatmap_points SET sport_type = 'gravel_cycling', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'gravel_cycling';
UPDATE heatmap_points SET sport_type = 'indoor_cycling', sub_type = NULL WHERE sport_type = 'cycling' AND sub_type = 'indoor_cycling';
UPDATE heatmap_points SET sport_type = 'trail_running', sub_type = NULL WHERE sport_type = 'running' AND sub_type = 'trail';
UPDATE heatmap_points SET sport_type = 'treadmill_running', sub_type = NULL WHERE sport_type = 'running' AND sub_type = 'indoor_running';
UPDATE heatmap_points SET sport_type = 'lap_swimming', sub_type = NULL WHERE sport_type = 'swimming' AND sub_type = 'lap_swimming';
UPDATE heatmap_points SET sport_type = 'open_water_swimming', sub_type = NULL WHERE sport_type = 'swimming' AND sub_type = 'open_water';

UPDATE segment_efforts SET sport_type = 'mountain_biking', sub_type = 'generic' WHERE sport_type = 'cycling' AND sub_type = 'mountain';
UPDATE segment_efforts SET sport_type = 'gravel_cycling', sub_type = 'generic' WHERE sport_type = 'cycling' AND sub_type = 'gravel_cycling';
UPDATE segment_efforts SET sport_type = 'indoor_cycling', sub_type = 'generic' WHERE sport_type = 'cycling' AND sub_type = 'indoor_cycling';
UPDATE segment_efforts SET sport_type = 'trail_running', sub_type = 'generic' WHERE sport_type = 'running' AND sub_type = 'trail';
UPDATE segment_efforts SET sport_type = 'treadmill_running', sub_type = 'generic' WHERE sport_type = 'running' AND sub_type = 'indoor_running';
UPDATE segment_efforts SET sport_type = 'lap_swimming', sub_type = 'generic' WHERE sport_type = 'swimming' AND sub_type = 'lap_swimming';
UPDATE segment_efforts SET sport_type = 'open_water_swimming', sub_type = 'generic' WHERE sport_type = 'swimming' AND sub_type = 'open_water';

UPDATE planned_routes SET sport_type = 'mountain_biking' WHERE sport_type = 'cycling' AND sub_type = 'mountain';
UPDATE planned_routes SET sport_type = 'gravel_cycling' WHERE sport_type = 'cycling' AND sub_type = 'gravel_cycling';
UPDATE planned_routes SET sport_type = 'indoor_cycling' WHERE sport_type = 'cycling' AND sub_type = 'indoor_cycling';
UPDATE planned_routes SET sport_type = 'trail_running' WHERE sport_type = 'running' AND sub_type = 'trail';
UPDATE planned_routes SET sport_type = 'treadmill_running' WHERE sport_type = 'running' AND sub_type = 'indoor_running';
UPDATE planned_routes SET sport_type = 'lap_swimming' WHERE sport_type = 'swimming' AND sub_type = 'lap_swimming';
UPDATE planned_routes SET sport_type = 'open_water_swimming' WHERE sport_type = 'swimming' AND sub_type = 'open_water';

ALTER TABLE planned_routes DROP COLUMN IF EXISTS sub_type;
ALTER TABLE routing_profiles DROP COLUMN IF EXISTS predicates;
ALTER TABLE routing_profiles DROP COLUMN IF EXISTS modifier_filters;

DROP TABLE IF EXISTS segments_legacy_sub_type_hints;
DROP FUNCTION IF EXISTS fn_validate_modifier_filters(JSONB);
DROP FUNCTION IF EXISTS fn_validate_predicates(JSONB);
