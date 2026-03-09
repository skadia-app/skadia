CREATE TABLE terra_incognita_cache (
    user_id       TEXT        NOT NULL PRIMARY KEY,
    radius_m      INT         NOT NULL,
    rings         TEXT[]      NOT NULL,
    area_km2      FLOAT8      NOT NULL DEFAULT 0,
    heatmap_count INT         NOT NULL DEFAULT 0,
    computed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
