-- Garmin Stamina (undocumented record fields 137/138) and GAP (field 140)
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS potential_stamina SMALLINT;
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS stamina SMALLINT;
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS gap_speed_ms REAL;
