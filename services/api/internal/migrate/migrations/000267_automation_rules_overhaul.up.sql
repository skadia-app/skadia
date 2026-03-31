-- 000267_automation_rules_overhaul.up.sql
-- Adds trigger_event, stop_on_match, apply_retroactively to automation_rules

ALTER TABLE automation_rules
    ADD COLUMN IF NOT EXISTS trigger_event TEXT NOT NULL DEFAULT 'on_import',
    ADD COLUMN IF NOT EXISTS stop_on_match BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS apply_retroactively BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS last_evaluated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_matched_count INTEGER DEFAULT 0;

-- All existing user rules should apply retroactively (preserve current behavior)
UPDATE automation_rules SET apply_retroactively = true WHERE is_system = false;

-- Constraint for trigger_event values
ALTER TABLE automation_rules
    ADD CONSTRAINT chk_trigger_event CHECK (
        trigger_event IN ('on_import', 'on_enrichment_complete', 'on_manual_apply')
    );
