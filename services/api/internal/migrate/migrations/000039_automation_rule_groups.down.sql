ALTER TABLE automation_rules
  DROP COLUMN IF EXISTS condition_groups,
  DROP COLUMN IF EXISTS conditions_logic;
