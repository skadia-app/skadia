ALTER TABLE gear
  ADD COLUMN IF NOT EXISTS default_sport_predicates JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE gear
SET default_sport_predicates = COALESCE((
  SELECT jsonb_agg(predicate ORDER BY ord)
  FROM (
    SELECT ord,
           CASE btrim(sport_type)
             WHEN 'mountain_biking' THEN jsonb_build_object('sport_type', 'cycling', 'sub_type', 'mountain')
             WHEN 'road_cycling' THEN jsonb_build_object('sport_type', 'cycling', 'sub_type', 'road')
             WHEN 'gravel_cycling' THEN jsonb_build_object('sport_type', 'cycling', 'sub_type', 'gravel_cycling')
             WHEN 'indoor_cycling' THEN jsonb_build_object('sport_type', 'cycling', 'sub_type', 'indoor_cycling')
             WHEN 'trail_running' THEN jsonb_build_object('sport_type', 'running', 'sub_type', 'trail')
             WHEN 'treadmill_running' THEN jsonb_build_object('sport_type', 'running', 'sub_type', 'treadmill')
             WHEN 'lap_swimming' THEN jsonb_build_object('sport_type', 'swimming', 'sub_type', 'lap_swimming')
             WHEN 'open_water_swimming' THEN jsonb_build_object('sport_type', 'swimming', 'sub_type', 'open_water')
             WHEN 'e_biking' THEN jsonb_build_object(
               'sport_type', 'cycling',
               'modifiers', jsonb_build_object('is_electric', true)
             )
             WHEN 'e_bike' THEN jsonb_build_object(
               'sport_type', 'cycling',
               'modifiers', jsonb_build_object('is_electric', true)
             )
             WHEN 'e_mountain_biking' THEN jsonb_build_object(
               'sport_type', 'cycling',
               'sub_type', 'mountain',
               'modifiers', jsonb_build_object('is_electric', true)
             )
             ELSE jsonb_build_object('sport_type', btrim(sport_type))
           END AS predicate
    FROM unnest(COALESCE(default_for_sports, ARRAY[]::text[])) WITH ORDINALITY AS defaults(sport_type, ord)
    WHERE btrim(sport_type) <> ''
  ) mapped
  WHERE predicate IS NOT NULL
    AND predicate->>'sport_type' <> 'unknown'
    AND fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
      'sport',
      predicate->>'sport_type',
      'sub_type',
      predicate->>'sub_type'
    ))))
), '[]'::jsonb)
WHERE default_sport_predicates = '[]'::jsonb
  AND COALESCE(cardinality(default_for_sports), 0) > 0;
