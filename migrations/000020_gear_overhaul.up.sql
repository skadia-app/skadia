-- Gear Overhaul: Status-System, Komponenten-Transfers, Kettenrotation, Service-Events, Presets

-- ============================================================
-- 1. gear: Neue Spalten + Status-Migration
-- ============================================================
ALTER TABLE gear ADD COLUMN IF NOT EXISTS total_elevation_gain_m DOUBLE PRECISION DEFAULT 0;
ALTER TABLE gear ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE gear ADD CONSTRAINT gear_status_check CHECK (status IN ('active', 'stored', 'retired'));

-- Migriere bestehende is_retired-Daten
UPDATE gear SET status = 'retired' WHERE is_retired = true;

-- ============================================================
-- 2. gear_components: Neue Spalten + Status-Migration
-- ============================================================
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS total_elevation_gain_m DOUBLE PRECISION DEFAULT 0;
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS total_time_s BIGINT DEFAULT 0;
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS purchase_price NUMERIC(10,2);
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS purchase_date DATE;
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS weight_g DOUBLE PRECISION;
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS serial_number TEXT;
ALTER TABLE gear_components ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE gear_components ADD CONSTRAINT gear_components_status_check CHECK (status IN ('active', 'stored', 'retired'));

-- Migriere bestehende is_retired-Daten
UPDATE gear_components SET status = 'retired' WHERE is_retired = true;

-- ============================================================
-- 3. component_assignments: Komponenten-Transfers zwischen Raedern
-- ============================================================
CREATE TABLE component_assignments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_id      UUID NOT NULL REFERENCES gear_components(id) ON DELETE CASCADE,
    gear_id           UUID NOT NULL REFERENCES gear(id) ON DELETE CASCADE,
    installed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_at        TIMESTAMPTZ,
    distance_m        DOUBLE PRECISION DEFAULT 0,
    elevation_gain_m  DOUBLE PRECISION DEFAULT 0,
    time_s            BIGINT DEFAULT 0,
    notes             TEXT
);

CREATE INDEX ON component_assignments (component_id);
CREATE INDEX ON component_assignments (gear_id);

-- ============================================================
-- 4. chain_rotation_groups: Kettenrotations-Gruppen
-- ============================================================
CREATE TABLE chain_rotation_groups (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    gear_id           UUID NOT NULL REFERENCES gear(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    swap_interval_km  DOUBLE PRECISION,
    swap_interval_hm  DOUBLE PRECISION,
    created_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ON chain_rotation_groups (user_id);
CREATE INDEX ON chain_rotation_groups (gear_id);

-- ============================================================
-- 5. chain_rotation_members: Mitglieder einer Rotationsgruppe
-- ============================================================
CREATE TABLE chain_rotation_members (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id                UUID NOT NULL REFERENCES chain_rotation_groups(id) ON DELETE CASCADE,
    component_id            UUID NOT NULL REFERENCES gear_components(id) ON DELETE CASCADE,
    position                INTEGER NOT NULL,
    is_active               BOOLEAN DEFAULT false,
    total_distance_m        DOUBLE PRECISION DEFAULT 0,
    total_elevation_gain_m  DOUBLE PRECISION DEFAULT 0,
    last_swapped_at         TIMESTAMPTZ
);

CREATE UNIQUE INDEX ON chain_rotation_members (group_id, component_id);

-- ============================================================
-- 6. gear_service_events: Durchgefuehrte Wartungen
-- ============================================================
CREATE TABLE gear_service_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_id    UUID NOT NULL REFERENCES gear_components(id) ON DELETE CASCADE,
    interval_id     UUID REFERENCES gear_service_intervals(id) ON DELETE SET NULL,
    performed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    distance_at_m   DOUBLE PRECISION,
    description     TEXT,
    cost            NUMERIC(10,2),
    notes           TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ON gear_service_events (component_id);
CREATE INDEX ON gear_service_events (interval_id);

-- ============================================================
-- 7. component_type_presets: Vordefinierte Komponententypen
-- ============================================================
CREATE TABLE component_type_presets (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    TEXT NOT NULL,
    category                TEXT NOT NULL,
    gear_type               TEXT NOT NULL,
    default_interval_type   TEXT,
    default_interval_value  DOUBLE PRECISION,
    track_elevation         BOOLEAN DEFAULT false,
    sort_order              INTEGER DEFAULT 0
);

-- Seed: Antrieb (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Kette',           'drivetrain', 'bike', 'distance_m', 5000000,  true,  1),
    ('Kettenblatt',     'drivetrain', 'bike', 'distance_m', 15000000, false, 2),
    ('Kassette',        'drivetrain', 'bike', 'distance_m', 15000000, false, 3),
    ('Schaltwerk',      'drivetrain', 'bike', 'distance_m', 30000000, false, 4),
    ('Umwerfer',        'drivetrain', 'bike', 'distance_m', 30000000, false, 5),
    ('Schaltzug',       'drivetrain', 'bike', 'distance_m', 10000000, false, 6),
    ('Schaltauge',      'drivetrain', 'bike', NULL,         NULL,     false, 7),
    ('Tretlager',       'drivetrain', 'bike', 'distance_m', 20000000, false, 8),
    ('Kurbel',          'drivetrain', 'bike', NULL,         NULL,     false, 9),
    ('Pedale',          'drivetrain', 'bike', NULL,         NULL,     false, 10);

-- Seed: Bremsen (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Bremsbelag vorne',    'braking', 'bike', 'distance_m', 5000000,  true,  1),
    ('Bremsbelag hinten',   'braking', 'bike', 'distance_m', 5000000,  true,  2),
    ('Bremsscheibe vorne',  'braking', 'bike', 'distance_m', 20000000, false, 3),
    ('Bremsscheibe hinten', 'braking', 'bike', 'distance_m', 20000000, false, 4),
    ('Bremsleitung',        'braking', 'bike', 'elapsed_days', 730,    false, 5),
    ('Bremsentleerung',     'braking', 'bike', 'elapsed_days', 365,    false, 6);

-- Seed: Laufraeder (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Laufrad vorne',   'wheels', 'bike', NULL,         NULL,     false, 1),
    ('Laufrad hinten',  'wheels', 'bike', NULL,         NULL,     false, 2),
    ('Reifen vorne',    'wheels', 'bike', 'distance_m', 8000000,  false, 3),
    ('Reifen hinten',   'wheels', 'bike', 'distance_m', 6000000,  false, 4),
    ('Schlauch vorne',  'wheels', 'bike', NULL,         NULL,     false, 5),
    ('Schlauch hinten', 'wheels', 'bike', NULL,         NULL,     false, 6),
    ('Felgenband',      'wheels', 'bike', NULL,         NULL,     false, 7),
    ('Speichen',        'wheels', 'bike', NULL,         NULL,     false, 8),
    ('Nabe vorne',      'wheels', 'bike', 'distance_m', 20000000, false, 9),
    ('Nabe hinten',     'wheels', 'bike', 'distance_m', 20000000, false, 10),
    ('Freilauf',        'wheels', 'bike', 'distance_m', 20000000, false, 11);

-- Seed: Rahmen & Cockpit (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Rahmen',          'frame', 'bike', NULL, NULL, false, 1),
    ('Gabel',           'frame', 'bike', NULL, NULL, false, 2),
    ('Lenker',          'frame', 'bike', NULL, NULL, false, 3),
    ('Vorbau',          'frame', 'bike', NULL, NULL, false, 4),
    ('Sattel',          'frame', 'bike', NULL, NULL, false, 5),
    ('Sattelstütze',    'frame', 'bike', NULL, NULL, false, 6),
    ('Steuersatz',      'frame', 'bike', 'distance_m', 20000000, false, 7),
    ('Lenkerband',      'frame', 'bike', 'distance_m', 5000000,  false, 8);

-- Seed: Federung (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Federgabel',          'suspension', 'bike', 'time_s', 6307200,  false, 1),
    ('Daempfer',            'suspension', 'bike', 'time_s', 6307200,  false, 2),
    ('Vario-Sattelstütze',  'suspension', 'bike', 'time_s', 6307200,  false, 3);

-- Seed: E-Bike (Fahrrad)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Akku',    'ebike', 'bike', NULL, NULL, false, 1),
    ('Motor',   'ebike', 'bike', NULL, NULL, false, 2),
    ('Display', 'ebike', 'bike', NULL, NULL, false, 3);

-- Seed: Schuhe (Laufen)
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Sohle',      'shoes', 'shoes', 'distance_m', 800000,  false, 1),
    ('Einlegesohle','shoes', 'shoes', 'distance_m', 500000,  false, 2),
    ('Schnuersenkel','shoes','shoes', NULL,         NULL,     false, 3);

-- Seed: Ski
INSERT INTO component_type_presets (name, category, gear_type, default_interval_type, default_interval_value, track_elevation, sort_order) VALUES
    ('Belag',    'ski', 'ski', 'elapsed_days', 365, false, 1),
    ('Kanten',   'ski', 'ski', 'elapsed_days', 365, false, 2),
    ('Bindung',  'ski', 'ski', 'elapsed_days', 365, false, 3),
    ('Felle',    'ski', 'ski', NULL,           NULL, false, 4);
