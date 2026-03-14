-- 000159_osm_pois_mixed_geometry.up.sql
-- Widen osm_pois.geom from Point-only to mixed Geometry (Point/Polygon/MultiPolygon)
-- for closed-way and relation POIs. GIST indexes work identically on mixed geometries.
--
-- HINWEIS: Diese Migration wirkt nur im Zeitfenster zwischen API-Deployment und dem
-- nächsten OSM-Import. osm2pgsql --create Mode droppt die Tabelle und legt sie mit
-- dem Schema aus define_table neu an. Die Migration sichert ab, dass Consumer-Code
-- mit ST_Centroid() schon korrekt funktioniert bevor der nächste Import läuft.
ALTER TABLE osm_pois
  ALTER COLUMN geom TYPE geometry(Geometry, 4326);
