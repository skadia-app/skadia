CREATE OR REPLACE FUNCTION source_type_priority(src TEXT) RETURNS INT IMMUTABLE LANGUAGE sql AS $$
    SELECT CASE src
        WHEN 'garmin_fit' THEN 50
        WHEN 'garmin'     THEN 40
        WHEN 'strava'     THEN 30
        WHEN 'komoot'     THEN 20
        WHEN 'manual'     THEN 10
        ELSE 0
    END;
$$;
