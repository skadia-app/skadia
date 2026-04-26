-- 000330_backfill_geo_tag_coords.up.sql
--
-- Root cause: the INSERT paths in geo_enrich.go (enrichGeoTagsSummits,
-- enrichGeoTagsSettlements) and stops.go (gastronomy) populated
-- map_point_lat/map_point_lon correctly but never wrote the same
-- coordinates into lat/lng. The frontend's TrackMap filters summit/
-- settlement markers on t.lat != null, so no map pins were drawn even
-- though the Pills ("1 Gipfel, 3 Ortschaften") showed correct counts.
--
-- Fix: the INSERT paths now write lat/lng from the same OSM centroid
-- values as map_point_lat/map_point_lon. This migration back-fills the
-- existing rows where map_point_lat is set but lat is still NULL
-- (point-type categories: summit, pass, settlement, gastronomy).
-- Area-type rows (forest, still_water, flowing_water, border) have no
-- map_point_lat and correctly remain without lat/lng.

UPDATE activity_geo_tags
SET lat = map_point_lat,
    lng = map_point_lon
WHERE map_point_lat IS NOT NULL
  AND lat IS NULL;
