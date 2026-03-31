-- 000267_automation_rules_overhaul.down.sql
ALTER TABLE automation_rules
    DROP CONSTRAINT IF EXISTS chk_trigger_event,
    DROP COLUMN IF EXISTS trigger_event,
    DROP COLUMN IF EXISTS stop_on_match,
    DROP COLUMN IF EXISTS apply_retroactively,
    DROP COLUMN IF EXISTS last_evaluated_at,
    DROP COLUMN IF EXISTS last_matched_count;
