-- 000338_user_map_layers_role_source.down.sql
-- Reverts the role/source_kind/style_url additions. Drops any vector_style rows
-- because url_template would need to be backfilled from style_url, which has no
-- raster equivalent — destructive but consistent with the schema invariant.

DELETE FROM user_map_layers WHERE source_kind = 'vector_style';

ALTER TABLE user_map_layers
    DROP CONSTRAINT IF EXISTS user_map_layers_url_consistency;

ALTER TABLE user_map_layers
    ALTER COLUMN url_template SET NOT NULL;

ALTER TABLE user_map_layers
    DROP CONSTRAINT IF EXISTS user_map_layers_style_url_length,
    DROP COLUMN IF EXISTS style_url,
    DROP COLUMN IF EXISTS source_kind,
    DROP COLUMN IF EXISTS role;
