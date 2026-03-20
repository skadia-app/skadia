-- Revert map_section back to LineString (drops any MultiLineString rows)
ALTER TABLE activity_geo_tags
    ALTER COLUMN map_section TYPE GEOMETRY(LineString, 4326)
        USING CASE
            WHEN GeometryType(map_section) = 'LINESTRING' THEN map_section::GEOMETRY(LineString, 4326)
            ELSE NULL
        END;
