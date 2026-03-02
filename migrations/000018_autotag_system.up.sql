-- Autotag-System: Erweitere tags-Tabelle um Autotag-Felder

ALTER TABLE tags ADD COLUMN IF NOT EXISTS is_auto BOOLEAN DEFAULT false;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS conditions JSONB;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS description TEXT;
