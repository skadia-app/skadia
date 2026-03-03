-- Normalize legacy sport_type values to canonical Garmin FIT SDK names.
-- parsed_sport_type is intentionally NOT changed (preserves original file values).

-- activities: normalize sport_type + sub_type
UPDATE activities SET sport_type = 'strength_training' WHERE sport_type = 'strength';
UPDATE activities SET sport_type = 'alpine_skiing' WHERE sport_type = 'skiing';
UPDATE activities SET sport_type = 'cycling', sub_type = COALESCE(NULLIF(sub_type, ''), 'mountain')
  WHERE sport_type = 'mountain_biking';
UPDATE activities SET sport_type = 'cycling', sub_type = COALESCE(NULLIF(sub_type, ''), 'gravel_cycling')
  WHERE sport_type = 'gravel_cycling';
UPDATE activities SET sport_type = 'running', sub_type = COALESCE(NULLIF(sub_type, ''), 'trail')
  WHERE sport_type = 'trail_running';
UPDATE activities SET sport_type = 'swimming', sub_type = COALESCE(NULLIF(sub_type, ''), 'open_water')
  WHERE sport_type = 'open_water_swimming';
UPDATE activities SET sport_type = 'swimming', sub_type = COALESCE(NULLIF(sub_type, ''), 'lap_swimming')
  WHERE sport_type = 'lap_swimming';
UPDATE activities SET sport_type = 'running', sub_type = COALESCE(NULLIF(sub_type, ''), 'treadmill')
  WHERE sport_type = 'treadmill_running';
UPDATE activities SET sport_type = 'cycling', sub_type = COALESCE(NULLIF(sub_type, ''), 'indoor_cycling')
  WHERE sport_type = 'indoor_cycling';
UPDATE activities SET sport_type = 'training', sub_type = 'yoga'
  WHERE sport_type = 'yoga';

-- heatmap_lines: normalize sport_type
UPDATE heatmap_lines SET sport_type = 'strength_training' WHERE sport_type = 'strength';
UPDATE heatmap_lines SET sport_type = 'alpine_skiing' WHERE sport_type = 'skiing';
UPDATE heatmap_lines SET sport_type = 'cycling' WHERE sport_type = 'mountain_biking';
UPDATE heatmap_lines SET sport_type = 'cycling' WHERE sport_type = 'gravel_cycling';
UPDATE heatmap_lines SET sport_type = 'running' WHERE sport_type = 'trail_running';
UPDATE heatmap_lines SET sport_type = 'swimming' WHERE sport_type = 'open_water_swimming';
UPDATE heatmap_lines SET sport_type = 'swimming' WHERE sport_type = 'lap_swimming';
UPDATE heatmap_lines SET sport_type = 'running' WHERE sport_type = 'treadmill_running';
UPDATE heatmap_lines SET sport_type = 'cycling' WHERE sport_type = 'indoor_cycling';
UPDATE heatmap_lines SET sport_type = 'training' WHERE sport_type = 'yoga';

-- heatmap_points: normalize sport_type
UPDATE heatmap_points SET sport_type = 'strength_training' WHERE sport_type = 'strength';
UPDATE heatmap_points SET sport_type = 'alpine_skiing' WHERE sport_type = 'skiing';
UPDATE heatmap_points SET sport_type = 'cycling' WHERE sport_type = 'mountain_biking';
UPDATE heatmap_points SET sport_type = 'cycling' WHERE sport_type = 'gravel_cycling';
UPDATE heatmap_points SET sport_type = 'running' WHERE sport_type = 'trail_running';
UPDATE heatmap_points SET sport_type = 'swimming' WHERE sport_type = 'open_water_swimming';
UPDATE heatmap_points SET sport_type = 'swimming' WHERE sport_type = 'lap_swimming';
UPDATE heatmap_points SET sport_type = 'running' WHERE sport_type = 'treadmill_running';
UPDATE heatmap_points SET sport_type = 'cycling' WHERE sport_type = 'indoor_cycling';
UPDATE heatmap_points SET sport_type = 'training' WHERE sport_type = 'yoga';

-- segment_efforts: normalize sport_type
UPDATE segment_efforts SET sport_type = 'strength_training' WHERE sport_type = 'strength';
UPDATE segment_efforts SET sport_type = 'alpine_skiing' WHERE sport_type = 'skiing';
UPDATE segment_efforts SET sport_type = 'cycling' WHERE sport_type = 'mountain_biking';
UPDATE segment_efforts SET sport_type = 'cycling' WHERE sport_type = 'gravel_cycling';
UPDATE segment_efforts SET sport_type = 'running' WHERE sport_type = 'trail_running';
UPDATE segment_efforts SET sport_type = 'swimming' WHERE sport_type = 'open_water_swimming';
UPDATE segment_efforts SET sport_type = 'swimming' WHERE sport_type = 'lap_swimming';
UPDATE segment_efforts SET sport_type = 'running' WHERE sport_type = 'treadmill_running';
UPDATE segment_efforts SET sport_type = 'cycling' WHERE sport_type = 'indoor_cycling';
UPDATE segment_efforts SET sport_type = 'training' WHERE sport_type = 'yoga';
