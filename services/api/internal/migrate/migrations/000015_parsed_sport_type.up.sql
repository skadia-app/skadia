-- Referenzwerte fuer den vom Parser erkannten Sporttyp.
-- Damit kann sport_type_is_manual korrekt gesetzt werden:
-- nur true wenn der manuelle Wert vom geparsten abweicht.
ALTER TABLE activities ADD COLUMN IF NOT EXISTS parsed_sport_type TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS parsed_sub_type TEXT;

-- Backfill: Fuer alle Aktivitaeten wo sport_type_is_manual=false
-- ist der aktuelle sport_type der geparste Wert.
UPDATE activities SET
  parsed_sport_type = sport_type,
  parsed_sub_type = sub_type
WHERE COALESCE(sport_type_is_manual, false) = false;
