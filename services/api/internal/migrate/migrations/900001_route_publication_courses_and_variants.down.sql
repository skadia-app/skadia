ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_access_scope_visibility_check;
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_visibility_check;
UPDATE segments SET visibility = 'private', access_scope_id = NULL WHERE visibility = 'shared';
ALTER TABLE segments ADD CONSTRAINT segments_visibility_check CHECK (visibility IN ('public', 'private'));
DROP INDEX IF EXISTS idx_segments_shared_access_scope;
ALTER TABLE segments DROP COLUMN IF EXISTS access_scope_id;

DROP TABLE IF EXISTS route_publication_events;
DROP TABLE IF EXISTS route_course_profile_projection;
DROP TABLE IF EXISTS route_profile_snapshots;
DROP TABLE IF EXISTS route_course_relationships;
DROP TABLE IF EXISTS route_relationships;
DROP TABLE IF EXISTS route_course_publications;
DROP TABLE IF EXISTS route_publication_privacy_reviews;
DROP TABLE IF EXISTS planned_route_snapshots;
DROP TABLE IF EXISTS object_access_grants;
DROP TABLE IF EXISTS object_access_scopes;
