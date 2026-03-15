-- Migration 000166: change hotfolder_enabled default to true
-- The hotfolder is opt-out, not opt-in.
-- Existing users that had the old default (false) are enabled automatically,
-- since the toggle had no effect on the running watcher before this change.
ALTER TABLE users ALTER COLUMN hotfolder_enabled SET DEFAULT true;
UPDATE users SET hotfolder_enabled = true WHERE hotfolder_enabled = false;
