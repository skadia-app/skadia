-- ============================================================
-- 000152: OSM Route Relations (hiking, cycling, mtb, ski, etc.)
-- Named route relations from OSM — Wanderwege, Radrouten, etc.
-- ============================================================

CREATE TABLE osm_routes (
    id              BIGSERIAL PRIMARY KEY,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    ref             TEXT,
    route           TEXT NOT NULL,        -- hiking|bicycle|mtb|foot|ski|running|ferry|via_ferrata
    network         TEXT,                 -- iwn/nwn/rwn/lwn (hiking) or icn/ncn/rcn/lcn (cycling)
    operator        TEXT,
    colour          TEXT,                 -- #RRGGBB or named color from OSM
    osmc_symbol     TEXT,                 -- raw osmc:symbol string (e.g. "red:red:white_bar")
    distance        REAL,                 -- declared distance in km (parsed from tags, may be NULL)
    ascent          INTEGER,              -- declared total ascent in meters
    descent         INTEGER,              -- declared total descent in meters
    description     TEXT,
    website         TEXT,
    tags            JSONB,
    geom            GEOMETRY(MultiLineString, 4326) NOT NULL
);

-- Geometry indexes
CREATE INDEX idx_osm_routes_geom    ON osm_routes USING GIST(geom);
CREATE INDEX idx_osm_routes_geog    ON osm_routes USING GIST((geom::geography));
CREATE INDEX idx_osm_routes_route   ON osm_routes(route);
CREATE INDEX idx_osm_routes_network ON osm_routes(network) WHERE network IS NOT NULL;
CREATE INDEX idx_osm_routes_osm_id  ON osm_routes(osm_id);

-- lz4 TOAST compression
ALTER TABLE osm_routes ALTER COLUMN geom SET COMPRESSION lz4;
ALTER TABLE osm_routes ALTER COLUMN tags SET COMPRESSION lz4;

-- Read access for skadia_readonly (SQL Agent / RLS)
GRANT SELECT ON osm_routes TO skadia_readonly;
