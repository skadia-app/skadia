ALTER TABLE gear
  ADD COLUMN IF NOT EXISTS default_sport_predicates JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE gear
SET default_sport_predicates = COALESCE((
  SELECT jsonb_agg(jsonb_build_object('sport_type', sport_type) ORDER BY ord)
  FROM unnest(COALESCE(default_for_sports, ARRAY[]::text[])) WITH ORDINALITY AS defaults(sport_type, ord)
  WHERE btrim(sport_type) <> ''
), '[]'::jsonb)
WHERE default_sport_predicates = '[]'::jsonb
  AND COALESCE(cardinality(default_for_sports), 0) > 0;
