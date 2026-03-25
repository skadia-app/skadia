## CHANGELOG.md
**Delta:** Add Unified ShadowFS feature entry under Unreleased

## docs/03_CORE_BACKEND.md — ShadowFS
**Suche nach:** Storage / File / Shadow
**Delta:** New section: Unified ShadowFS (LocalShadowFS + S3ShadowFS), user_files registry, per-type schemas, Library Watcher, Reconciliation, GDPR integration

## docs/08_API.md — New endpoints
**Suche nach:** /api/v1/
**Delta:** GET/PUT /settings/file-schemas, GET/PUT /settings/library-dir, GET /files/{fileID}, GET /admin/storage, POST /admin/storage/populate-registry, POST /admin/storage/migrate, POST /admin/storage/reconcile

## docs/01_INFRA.md — Docker changes
**Suche nach:** docker-compose / Garage
**Delta:** STORAGE_BACKEND=local default, Garage commented out, LIBRARY_BASE_PATH bind mount
