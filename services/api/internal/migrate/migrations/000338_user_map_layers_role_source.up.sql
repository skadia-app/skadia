-- 000338_user_map_layers_role_source.up.sql
-- Lift user_map_layers to feature parity with map_layers:
--   * role             ('base' | 'overlay') — choose tab vs. overlay placement
--   * source_kind      ('raster_tiles' | 'vector_style') — raster XYZ or MapLibre style URL
--   * style_url        — required when source_kind='vector_style', mirrors map_layers
--   * url_template now nullable (style_url path doesn't use it)
--   * url consistency CHECK analogous to map_layers

ALTER TABLE user_map_layers
    ADD COLUMN role        TEXT NOT NULL DEFAULT 'overlay'
        CHECK (role IN ('base','overlay')),
    ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'raster_tiles'
        CHECK (source_kind IN ('raster_tiles','vector_style')),
    ADD COLUMN style_url   TEXT,
    ADD CONSTRAINT user_map_layers_style_url_length CHECK (
        style_url IS NULL OR length(style_url) <= 2048
    );

ALTER TABLE user_map_layers
    ALTER COLUMN url_template DROP NOT NULL;

ALTER TABLE user_map_layers
    ADD CONSTRAINT user_map_layers_url_consistency CHECK (
        (source_kind = 'raster_tiles' AND url_template IS NOT NULL AND style_url IS NULL) OR
        (source_kind = 'vector_style' AND style_url    IS NOT NULL AND url_template IS NULL)
    );
