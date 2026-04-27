-- 000333_map_layers.up.sql
-- Adds three tables for map-layer management:
--   map_layers              — instance-wide URL-based sources (admin-managed)
--   system_layer_visibility — toggles + defaults for built-in vector + coverage layers
--   user_map_layers         — per-user custom tile URLs

-- 1. map_layers ---------------------------------------------------------------
CREATE TABLE map_layers (
    id              BIGSERIAL PRIMARY KEY,
    key             TEXT NOT NULL UNIQUE,
    role            TEXT NOT NULL CHECK (role IN ('base','overlay')),
    name            TEXT NOT NULL,
    source_kind     TEXT NOT NULL CHECK (source_kind IN ('raster_tiles','vector_style','protomaps_offline')),
    url_template    TEXT,
    style_url       TEXT,
    attribution     TEXT,
    min_zoom        SMALLINT,
    max_zoom        SMALLINT,
    default_opacity SMALLINT NOT NULL DEFAULT 100 CHECK (default_opacity BETWEEN 0 AND 100),
    is_system       BOOLEAN NOT NULL DEFAULT false,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    proxy_through_backend BOOLEAN NOT NULL DEFAULT false,
    requires_capability TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT map_layers_url_consistency CHECK (
        (source_kind = 'raster_tiles'      AND url_template IS NOT NULL AND style_url IS NULL)  OR
        (source_kind = 'vector_style'      AND style_url    IS NOT NULL AND url_template IS NULL) OR
        (source_kind = 'protomaps_offline' AND url_template IS NULL     AND style_url IS NULL)
    ),
    CONSTRAINT map_layers_name_length        CHECK (length(name)         BETWEEN 1 AND 200),
    CONSTRAINT map_layers_attribution_length CHECK (attribution IS NULL OR length(attribution) <= 500),
    CONSTRAINT map_layers_url_length         CHECK (url_template IS NULL OR length(url_template) <= 2048),
    CONSTRAINT map_layers_style_url_length   CHECK (style_url    IS NULL OR length(style_url)    <= 2048)
);

CREATE INDEX map_layers_role_sort ON map_layers(role, sort_order) WHERE enabled;

-- 2. system_layer_visibility --------------------------------------------------
CREATE TABLE system_layer_visibility (
    layer_key                     TEXT PRIMARY KEY,
    enabled                       BOOLEAN NOT NULL DEFAULT true,
    default_enabled_for_new_users BOOLEAN NOT NULL DEFAULT false,
    default_opacity               SMALLINT CHECK (default_opacity IS NULL OR default_opacity BETWEEN 0 AND 100),
    label_override                TEXT CHECK (label_override IS NULL OR length(label_override) BETWEEN 1 AND 100),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. user_map_layers ----------------------------------------------------------
-- users.id is UUID (000002_users.up.sql:2 — `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`).
CREATE TABLE user_map_layers (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    url_template    TEXT NOT NULL,
    attribution     TEXT,
    min_zoom        SMALLINT,
    max_zoom        SMALLINT,
    default_opacity SMALLINT NOT NULL DEFAULT 100 CHECK (default_opacity BETWEEN 0 AND 100),
    enabled         BOOLEAN NOT NULL DEFAULT true,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_map_layers_name_length        CHECK (length(name)         BETWEEN 1 AND 200),
    CONSTRAINT user_map_layers_attribution_length CHECK (attribution IS NULL OR length(attribution) <= 500),
    CONSTRAINT user_map_layers_url_length         CHECK (length(url_template) <= 2048)
);

CREATE INDEX user_map_layers_user ON user_map_layers(user_id, sort_order);
CREATE UNIQUE INDEX user_map_layers_user_name ON user_map_layers(user_id, lower(name));

-- 4. Updated-At Trigger (function set_updated_at() since migration 000205) ----
CREATE TRIGGER trg_map_layers_updated_at
    BEFORE UPDATE ON map_layers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_system_layer_visibility_updated_at
    BEFORE UPDATE ON system_layer_visibility
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_user_map_layers_updated_at
    BEFORE UPDATE ON user_map_layers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 5. Seed map_layers (7 system entries; keys identical to historic mapStyles.ts) -
INSERT INTO map_layers (key, role, name, source_kind, url_template, style_url, attribution, min_zoom, max_zoom, is_system, sort_order, requires_capability) VALUES
    ('osm',       'base', 'OpenStreetMap',     'raster_tiles',
        'https://tile.openstreetmap.org/{z}/{x}/{y}.png', NULL,
        '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
        0, 19, true, 1, NULL),
    ('topo',      'base', 'OpenTopoMap',       'raster_tiles',
        'https://tile.opentopomap.org/{z}/{x}/{y}.png', NULL,
        '&copy; <a href="https://opentopomap.org/about">OpenTopoMap</a> (CC-BY-SA)',
        0, 17, true, 2, NULL),
    ('cyclosm',   'base', 'CyclOSM',           'raster_tiles',
        'https://{switch:a,b,c}.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png', NULL,
        '&copy; <a href="https://www.cyclosm.org">CyclOSM</a>',
        0, 20, true, 3, NULL),
    ('satellite', 'base', 'Esri World Imagery','raster_tiles',
        'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}', NULL,
        '&copy; <a href="https://www.esri.com">Esri</a>, Maxar, Earthstar Geographics',
        0, 19, true, 4, NULL),
    ('dark',      'base', 'Carto Dark',        'raster_tiles',
        'https://{switch:a,b,c}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png', NULL,
        '&copy; <a href="https://carto.com/attributions">CARTO</a>',
        0, 20, true, 5, NULL),
    -- style_url is relative to the API origin; assumes Same-Origin frontend
    -- (mobile webview / cross-origin embedding would need adjustment — Future Consideration).
    ('local',     'base', 'Lokal (Vector)',    'vector_style',
        NULL, '/api/v1/tiles/style.json',
        'Lokale PMTiles',
        0, 16, true, 6, 'offline_map'),
    ('offline',   'base', 'Offline (Protomaps)', 'protomaps_offline',
        NULL, NULL,
        'Lokale PMTiles via @protomaps/basemaps',
        0, 16, true, 7, 'offline_map');

-- 6. Seed system_layer_visibility (14 entries: 6 vector + 8 coverage) ---------
INSERT INTO system_layer_visibility (layer_key, enabled, default_enabled_for_new_users, default_opacity) VALUES
    -- Vector overlays
    ('summits',         true, false, NULL),
    ('waters',          true, false, NULL),
    ('forests',         true, false, NULL),
    ('visited_pois',    true, false, NULL),
    ('amenities_tiles', true, false, NULL),
    ('segments_lines',  true, true,  NULL),
    -- Coverage sublayers (composite "nature")
    ('coverage_landuse',         true, false, 60),
    ('coverage_forest',          true, false, 60),
    ('coverage_water',           true, false, 60),
    ('coverage_river_proximity', true, false, 60),
    ('coverage_protected',       true, false, 60),
    -- Coverage sublayer (composite "noise")
    ('coverage_noise',           true, false, 70),
    -- Coverage sublayers (composite "trafficUrban")
    ('coverage_traffic',         true, false, 70),
    ('coverage_urbanization',    true, false, 70);

-- 7. Audit-Log entry (analog Mig 000326) ---------------------------------------
INSERT INTO auth_audit_log (user_id, event_type, details)
VALUES (NULL, 'migration', jsonb_build_object(
    'migration', '000333',
    'name', 'map_layers',
    'map_layers_seeded', 7,
    'system_layer_visibility_seeded', 14
));
