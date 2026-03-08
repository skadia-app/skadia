-- is_race wird nicht mehr benötigt — Wettkampf läuft über Tag-System
ALTER TABLE activities DROP COLUMN IF EXISTS is_race;
