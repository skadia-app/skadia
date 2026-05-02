-- WARNING: dev-only / pre-frontend-deploy reversal.
-- After deploy, planned_routes.version is read by the routes handler
-- (ETag emission) and required for every guarded UPDATE; dropping the
-- column would break PATCH /api/v1/routes/{id} at runtime.
ALTER TABLE planned_routes DROP COLUMN version;
