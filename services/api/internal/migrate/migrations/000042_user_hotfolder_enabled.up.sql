-- Migration 000041: persist hotfolder_enabled per user
-- The toggle in the UI now writes to the DB; the server reads this at request time.

ALTER TABLE users ADD COLUMN IF NOT EXISTS hotfolder_enabled BOOLEAN NOT NULL DEFAULT false;
