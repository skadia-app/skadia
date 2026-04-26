-- 000330_backfill_geo_tag_coords down
--
-- NOT REVERSIBLE.
--
-- The up migration backfilled lat/lng for activity_geo_tags rows whose
-- map_point_lat/map_point_lon were set but lat/lng were NULL. After this
-- migration runs, new code paths (geo_enrich.go, stops.go) write canonical
-- lat/lng on INSERT, so we cannot distinguish "filled by this migration"
-- from "written by current code". Reverting blindly would null out
-- production data; we choose data preservation over rollback symmetry.
--
-- If a hard rollback is needed, restore from a pre-migration backup.

SELECT 1; -- no-op
