-- Heatmap sub_type, Preset-Korrekturen/-Erweiterungen, Bike-Setups

-- ============================================================
-- 1. Heatmap: sub_type Spalte hinzufügen
-- ============================================================
ALTER TABLE heatmap_lines  ADD COLUMN IF NOT EXISTS sub_type TEXT;
ALTER TABLE heatmap_points ADD COLUMN IF NOT EXISTS sub_type TEXT;

-- Bestehende Daten backfillen
UPDATE heatmap_lines hl SET sub_type = a.sub_type
FROM activities a WHERE a.id = hl.activity_id AND a.sub_type IS NOT NULL AND hl.sub_type IS NULL;

UPDATE heatmap_points hp SET sub_type = a.sub_type
FROM activities a WHERE a.id = hp.activity_id AND a.sub_type IS NOT NULL AND hp.sub_type IS NULL;

-- ============================================================
-- 2. Preset-Korrekturen (Umlaute + inhaltlich)
-- ============================================================
UPDATE component_type_presets SET name = 'Dämpfer'         WHERE name = 'Daempfer';
UPDATE component_type_presets SET name = 'Schnürsenkel'    WHERE name = 'Schnuersenkel';
UPDATE component_type_presets SET name = 'Bremsflüssigkeit' WHERE name = 'Bremsentleerung';

-- ============================================================
-- 3. Neue Preset-Komponenten
-- ============================================================

-- Antrieb (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Schaltzugaußenhülle', 'drivetrain', 'bike', 'distance_m', 10000000, false, 11),
    ('Schalthebel',         'drivetrain', 'bike', NULL,         NULL,     false, 12),
    ('Cleats',              'drivetrain', 'bike', 'distance_m', 5000000,  false, 13),
    ('Kettenführung',       'drivetrain', 'bike', NULL,         NULL,     false, 14),
    ('Riemen',              'drivetrain', 'bike', NULL,         NULL,     false, 15),
    ('Nabenschaltung',      'drivetrain', 'bike', NULL,         NULL,     false, 16);

-- Bremsen (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Bremssattel vorne',  'braking', 'bike', NULL,         NULL, false, 7),
    ('Bremssattel hinten', 'braking', 'bike', NULL,         NULL, false, 8),
    ('Bremshebel',         'braking', 'bike', NULL,         NULL, false, 9),
    ('Bremszug',           'braking', 'bike', 'distance_m', 10000000, false, 10),
    ('Bremsbacke',         'braking', 'bike', 'distance_m', 5000000,  false, 11);

-- Laufräder (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Dichtmilch',       'wheels', 'bike', 'elapsed_days', 90,   false, 12),
    ('Tubeless-Ventil',  'wheels', 'bike', NULL,           NULL, false, 13),
    ('Felge',            'wheels', 'bike', NULL,           NULL, false, 14),
    ('Steckachse vorne', 'wheels', 'bike', NULL,           NULL, false, 15),
    ('Steckachse hinten','wheels', 'bike', NULL,           NULL, false, 16);

-- Rahmen & Cockpit (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Spacer',       'frame', 'bike', NULL, NULL, false, 9),
    ('Sattelklemme', 'frame', 'bike', NULL, NULL, false, 10),
    ('Griffe',       'frame', 'bike', 'distance_m', 5000000, false, 11),
    ('Barends',      'frame', 'bike', NULL, NULL, false, 12);

-- Federung (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Gabeldichtung',  'suspension', 'bike', 'time_s', 6307200, false, 4),
    ('Dämpferbuchse',  'suspension', 'bike', NULL,     NULL,    false, 5);

-- E-Bike (Ergänzungen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Geschwindigkeitssensor', 'ebike', 'bike', NULL, NULL, false, 4),
    ('Drehmomentsensor',       'ebike', 'bike', NULL, NULL, false, 5),
    ('Controller',             'ebike', 'bike', NULL, NULL, false, 6),
    ('Ladegerät',              'ebike', 'bike', NULL, NULL, false, 7);

-- NEU: Beleuchtung
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Scheinwerfer',  'lighting', 'bike', NULL, NULL, false, 1),
    ('Rücklicht',     'lighting', 'bike', NULL, NULL, false, 2),
    ('Nabendynamo',   'lighting', 'bike', NULL, NULL, false, 3);

-- NEU: Zubehör
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Schutzblech vorne',  'accessories', 'bike', NULL, NULL, false, 1),
    ('Schutzblech hinten', 'accessories', 'bike', NULL, NULL, false, 2),
    ('Gepäckträger',       'accessories', 'bike', NULL, NULL, false, 3),
    ('Ständer',            'accessories', 'bike', NULL, NULL, false, 4),
    ('Klingel',            'accessories', 'bike', NULL, NULL, false, 5),
    ('Flaschenhalter',     'accessories', 'bike', NULL, NULL, false, 6);

-- ============================================================
-- 4. Bike-Setups
-- ============================================================
CREATE TABLE bike_setups (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gear_id           UUID NOT NULL REFERENCES gear(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    rider_weight_kg   DECIMAL(5,1),
    intended_use      TEXT,
    notes             TEXT,
    suspension_front  JSONB,
    suspension_rear   JSONB,
    tires             JSONB,
    contact_points    JSONB,
    brakes            JSONB,
    drivetrain        JSONB,
    is_active         BOOLEAN DEFAULT false,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ON bike_setups (gear_id);

-- Trigger: nur ein aktives Setup pro Gear
CREATE OR REPLACE FUNCTION deactivate_other_setups() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_active = true THEN
        UPDATE bike_setups SET is_active = false
        WHERE gear_id = NEW.gear_id AND id != NEW.id AND is_active = true;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_deactivate_other_setups
    BEFORE INSERT OR UPDATE OF is_active ON bike_setups
    FOR EACH ROW EXECUTE FUNCTION deactivate_other_setups();
