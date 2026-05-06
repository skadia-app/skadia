ALTER TABLE gear
  DROP CONSTRAINT IF EXISTS gear_default_sport_predicates_valid;

ALTER TABLE gear
  DROP COLUMN IF EXISTS default_sport_predicates;

DROP FUNCTION IF EXISTS fn_validate_gear_default_predicates(JSONB);
