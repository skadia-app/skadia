-- Gruppenbasierte Bedingungslogik fuer Tags (AND/OR-Gruppen, Geofencing)

ALTER TABLE tags ADD COLUMN IF NOT EXISTS condition_groups JSONB;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS conditions_logic TEXT DEFAULT 'and';
