## services/api — Activity List Actions (Track A Backend)

**Suche nach:** bulk delete, export-zip, segment backfill
**Delta:**
- `services/api/internal/handler/recompute.go`: `doSegments := allScope || scope["segments"]` added; section 8c trigger updated to `doStreams || doSegments || allScope`
- `services/api/internal/database/segments.go`: new `ListActivityIDsForSegmentBackfill(ctx, userID, segmentID)` — reverse spatial query, finds GPS activities within 50m of new segment track
- `services/api/internal/handler/segments.go`: `CreateSegment` now launches backfill goroutine after returning 201; dereferences `act.SubType *string` safely
- `services/api/internal/database/database.go`: new `DeleteActivities(ctx, ids []string) (int64, error)` — bulk DELETE with dynamic IN-clause
- `services/api/internal/handler/activities.go`: new `DeleteActivities` handler for `DELETE /api/v1/activities` with JSON body `{"ids": [...]}`; max 500 IDs
- `services/api/internal/handler/export_zip.go` (NEW): `ExportActivitiesZip` handler for `POST /api/v1/activities/export-zip`; body `{"ids": [...], "format": "original|gpx|tcx|fit"}`; max 1000 activities; `exportActivityBytes` helper; `writeGPXBytes`/`writeTCXBytes`/`writeFITBytes` helpers; `bytesWriter` adapter; `jsonDecodeBody` helper
- `services/api/cmd/api/main.go`: routes added: `DELETE /activities`, `POST /activities/export-zip` (before `{id}` routes)

## CHANGELOG.md
- feat: add `segments` scope to `reprocessActivity` — allows re-running segment matching without full re-parse
- feat: segment backfill goroutine on `POST /segments` — matches new segment against existing GPS activities
- feat: `DELETE /api/v1/activities` — bulk delete up to 500 activities
- feat: `POST /api/v1/activities/export-zip` — ZIP export for up to 1000 activities in original/gpx/tcx/fit format
