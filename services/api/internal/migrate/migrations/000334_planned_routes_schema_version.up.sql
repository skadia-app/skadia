ALTER TABLE planned_routes
  ADD COLUMN route_schema_version smallint NOT NULL DEFAULT 0;

COMMENT ON COLUMN planned_routes.route_schema_version IS
  'Profile-Schema-Version. 0 = Pre-T0 (approach-exclusive, needs migration). 1 = T0 (approach-inclusive, full profile incl. VE-Tail/Approach/Spline).';
