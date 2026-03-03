ALTER TABLE automation_rules
  ADD COLUMN condition_groups JSONB,
  ADD COLUMN conditions_logic TEXT NOT NULL DEFAULT 'and';
