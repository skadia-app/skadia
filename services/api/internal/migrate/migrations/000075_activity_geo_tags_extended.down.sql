ALTER TABLE activity_geo_tags
    DROP COLUMN IF EXISTS map_type,
    DROP COLUMN IF EXISTS map_point_lat,
    DROP COLUMN IF EXISTS map_point_lon,
    DROP COLUMN IF EXISTS map_bbox_west,
    DROP COLUMN IF EXISTS map_bbox_south,
    DROP COLUMN IF EXISTS map_bbox_east,
    DROP COLUMN IF EXISTS map_bbox_north,
    DROP COLUMN IF EXISTS map_section,
    DROP COLUMN IF EXISTS length_m,
    DROP COLUMN IF EXISTS ele_m;
