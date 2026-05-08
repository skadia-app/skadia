ALTER TABLE gear
  ADD COLUMN IF NOT EXISTS default_sport_predicates JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE OR REPLACE FUNCTION fn_validate_gear_default_predicates(p JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
SET search_path = public
AS $$
WITH items AS (
  SELECT value AS predicate
  FROM jsonb_array_elements(CASE WHEN jsonb_typeof(p) = 'array' THEN p ELSE '[]'::jsonb END)
),
modifier_items AS (
  SELECT predicate, modifier.key, modifier.value
  FROM items
  CROSS JOIN LATERAL jsonb_each(COALESCE(predicate->'modifiers', '{}'::jsonb)) AS modifier(key, value)
  WHERE jsonb_typeof(predicate) = 'object'
)
SELECT
  COALESCE(jsonb_typeof(p) = 'array', false)
  AND NOT EXISTS (
    SELECT 1
    FROM items
    WHERE jsonb_typeof(predicate) <> 'object'
       OR EXISTS (
            SELECT 1
            FROM jsonb_object_keys(predicate) AS pred_keys(key)
            WHERE pred_keys.key NOT IN ('sport_type', 'sub_type', 'modifiers')
       )
       OR NOT (predicate ? 'sport_type')
       OR jsonb_typeof(predicate->'sport_type') <> 'string'
       OR predicate->>'sport_type' = 'unknown'
       OR (
            predicate ? 'sub_type'
            AND jsonb_typeof(predicate->'sub_type') <> 'string'
       )
       OR (
            predicate ? 'modifiers'
            AND jsonb_typeof(predicate->'modifiers') <> 'object'
       )
       OR NOT fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport',
            predicate->>'sport_type',
            'sub_type',
            predicate->>'sub_type'
       ))))
  )
  AND NOT EXISTS (
    SELECT 1
    FROM modifier_items
    WHERE key NOT IN ('is_electric', 'is_indoor', 'is_virtual')
       OR jsonb_typeof(value) <> 'boolean'
  );
$$;

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

ALTER TABLE gear
  DROP CONSTRAINT IF EXISTS gear_default_sport_predicates_valid,
  ADD CONSTRAINT gear_default_sport_predicates_valid
    CHECK (fn_validate_gear_default_predicates(default_sport_predicates)) NOT VALID;
ALTER TABLE gear VALIDATE CONSTRAINT gear_default_sport_predicates_valid;
