# Changelog

All notable user-facing, administrator-facing, security, operational and API-relevant changes are documented here.

---

## 2026-05-21

### Added

- Saved routes can now be explicitly published from the Route Planner as separate private, shared or public rankable courses backed by immutable route snapshots.
- Route publication APIs now support route snapshots, course publication, route forks, route relationships, published-course relationships, and published-course search by sport, difficulty, distance, elevation, visibility, and variants.
- Shared object access scopes can now be used by segment-backed course visibility and published-course search.
- Target details can now expose optional activity observations, reports, conditions, comments and ratings without adding overlay joins to normal segment/course search, matching or leaderboard reads.
- Checkpoint courses can now be created, matched, listed, exported and imported as rankable courses where the path between ordered controls is free.
- Public or shared segment/course creation from an activity now requires explicit confirmation when the source track intersects an active privacy zone; the requested geometry is recorded unchanged.
- Target object management now supports places, regions and collections, including collection members for segments/courses, published route courses, places, regions, privacy zones and tag zones.
- Governance scopes, roles and target bindings now provide a delegated-edit foundation for target objects and related map/course objects.

### Changed

- Planned routes and published courses now keep separate histories: editing or forking the source route does not silently rewrite an already published course.
- Route-derived course segments now follow the route-publication workflow instead of the generic segment publish workflow.
- Segment search and map overlays now distinguish line courses from checkpoint courses through `course_kind` while keeping normal segment requests line-course-only by default.
- Segment creation and editing now expose private, shared and public visibility, with shared segments tied to explicit access scopes.

### Fixed

- Revoking a shared access scope, or its last live view grant, now demotes attached shared segment-backed courses to private and refreshes their course visibility projections.
- Public and shared route publication now requires an explicit privacy-review state or owner confirmation instead of rewriting route names, descriptions, waypoints, geometry, or media pointers.
- Route publication privacy review now treats absent optional route components as safe, and the route planner confirmation covers the full route snapshot that will become visible.
- Route forks created from the planner now offer an open action that loads the fork back into the map planner.
- Shared target reports, observations and user content now reject access scopes that the submitting user cannot use.
- Segment/course detail responses now expose delegated report-management permissions only where the current UI can act on them.
- Target object and collection visibility now redacts owner-only external references, private collection members and private member bounds from non-owner responses.
- GDPR export, import and deletion now cover target objects, collections, governance audit state, target overlays and derived target-object projections.

---

## 2026-05-20

### Added

- Activity detail pages now support assigning and removing multiple gear items directly from the overview context card.
- Segment APIs now expose a compact `elevation_kind` classification and filter for flat, hilly, uphill and downhill segments.

### Changed

- Activity detail pages now use a calmer context card for location, recording device, recording time range, weather, tags, gear, platform links and best-effort highlights.
- Activity quick edit now groups sport modifiers, privacy settings and statistics exclusion separately; detailed privacy controls are available behind `Details`.
- Segment geometry changes now use timestamped versions. Existing efforts stay attached to the version and direction that matched the source activity, and sport eligibility is snapshotted per segment version.
- Segment version edits, publish actions and manual recomputes now use durable per-activity recalculation runs with visible progress only for explicit user actions.

### Fixed

- Segment creation now returns immediately while matching continues in the background, and the source activity refreshes its segment list quietly after saving.
- Segment recompute and matching now replace existing efforts idempotently, deduplicate grouped recording sources in leaderboards and activity segment lists, and use the activity's current sport classification during recompute.
- Segment rollbacks now preserve historical version rows without making retired versions active for matching, and repeated version-edit requests are rejected when the operation ID is reused with different input.
- Segment completion counts now include public completed traversals separately from rankable leaderboard rows.
- Activity detail best-effort highlights no longer show `Top 0%` for top-ranked efforts.

---

## 2026-05-19

### Changed

- The chat assistant now handles gear components and chain rotations as gear-specific write actions instead of treating components as separate top-level gear.

### Fixed

- Chat-created gear now rejects component type keys as top-level gear types and normalizes component labels before saving.
- Storage backend migration now uses the offline migration path with durable maintenance mode, stronger source/target validation, file-registry checks and clearer setup warnings before changing storage modes.

---

## 2026-05-18

### Added

- Added an admin System Status page with service readiness, feature availability, storage diagnostics, build provenance and a redacted diagnostic export.
- BRouter route results can now show way types, surfaces, SAC/MTB difficulty ranges, trail-visibility metadata and summit lists when the required route and OSM metadata are available.

### Changed

- Activity detail pages now use a split overview with a left context card, source tabs for grouped recordings, compact action buttons, earlier weather and tag placement, and deduplicated country flags when country detection is available.
- Activity, analytics, map heatmap and assistant filters can now filter activities by detected country codes, including cross-border activities when boundary enrichment is available.
- Build metadata now includes both commit time and build time for API, frontend and ML service diagnostics when available.
- Local Compose rebuilds can use `scripts/compose.sh` to refresh embedded build provenance before delegating to the selected container runtime.
- Route and segment lists now load additional entries automatically while scrolling instead of relying on one fixed-size list response.
- Route suggestions now expose only routing-relevant controls in the normal dialog, keep round-trip behavior implicit, and allow target distances up to 100 km.
- Routing profile editing now keeps non-routing modifiers out of the normal profile form while preserving electric-drive matching.
- Komoot recorded-activity imports are now documented as connector exports rather than original device FIT files, including the limitation that duplicate native imports stay the richer primary activity while Komoot imports become grouped sources or fusion candidates.

### Fixed

- Route suggestions are less likely to fail when many nearby candidates collapse into the same distance bucket.
- Route suggestions now keep surface, way-type and difficulty metadata when a suggestion is loaded into the planner.
- Broad cycling route suggestions now keep compatible cycling profiles available instead of forcing an unrelated fallback profile.
- Saved routes with empty or `outdoor` profile defaults now infer a compatible planner profile from the stored route sport before loading.
- System Status now checks all effective S3 buckets, marks diagnostic responses as non-cacheable, surfaces migration and GDPR artifact probe failures, and labels disabled Core-profile dependencies more accurately.
- The admin System Status page now shows localized status text, API/frontend/ML build provenance, and export error feedback.
- Komoot import progress and completion messages now match the selected import scope, such as route-only or activity-only runs.
- Bulk activity imports now document the delayed fusion-candidate refresh behavior so long imports do not appear to restart the same recompute job for every imported file.
- Activity detail overflow actions now retain fusion reversal, multisport ungrouping, sport-conflict resolution and scoped reprocessing after the overview redesign.
- Activity country lists now stay in sync after single-activity recompute, fusion creation and manual-location edits, and analytics country counts use the same detected country list as filters.
- Temporary Komoot credentials are no longer offered as saved credentials after a non-persistent import flow.

---

## 2026-05-17

### Added

- Chat assistant tools now expose capability metadata and the assistant chooses tool sets from the request intent before execution.

### Changed

- Local Compose installations now serve the built web UI from the API container on `localhost:8080`; development mode continues to use the separate Vite dev server.
- The reverse proxy container is now only enabled for server deployments and old local proxy/frontend containers are removed during `compose up --remove-orphans`.
- Summit collection is now part of Analytics instead of the Workshop area.
- Settings no longer exposes legacy importer shortcuts; imports are reached through the central import area.

### Fixed

- Capability checks now hide or disable analytics, summit, photo and routing surfaces more consistently when their required deployment features are unavailable.
- Bulk activity imports now coalesce fusion-candidate recalculation instead of restarting the same recompute job repeatedly while new activities are still arriving.
- The public-content guard allows documented provenance metadata while continuing to block private paths and sensitive local details.
- Static web asset serving now restores long-term cache headers for hashed assets and rejects symlink escapes from the configured frontend asset directory.
- TimescaleDB compression setup no longer offers an ineffective setup-time choice. Compression is managed by database migrations and the Admin UI.
- The TimescaleDB compression toggle now uses the correct stream time column and keeps the setting unchanged when disabling compression fails.
- Chat now shows compact tool progress and diagnostics, reports unavailable routing or weather tools more clearly, and rejects unplanned tool calls.
- Heatmap gear filters and restore/import paths now read the current gear-assignment table so primary gear assignments survive restores and map filtering stays correct.
- Summit collection filters and map markers now stay isolated from each other.
- Gear document processing now handles scanned PDFs and long page extraction requests more reliably, and reuses extracted Markdown during retries.
- Map sport filters now use the same sport-color palette as the rest of the map UI.

---

## 2026-05-16

### Fixed

- Deployment readiness checks now distinguish process liveness, service readiness, configured capabilities and missing local-data artifacts more consistently.
- Routing, map-layer, reprocessing and lift controls now respect runtime capability availability before showing or starting Full-only actions.
- ML media workers now wait or degrade cleanly when required storage is not ready instead of treating unavailable storage as a successful capability.
- Deployment smoke checks isolate test projects more safely and validate runtime, storage and published API readiness before reporting success.

---

## 2026-05-15

### Added

- Added selectable Core and Full setup profiles. Core runs without local OSM, DEM, coverage, offline-map, BadRouter and ML pipelines; Full enables local data features when their data is available.
- Added explicit Docker or Podman runtime selection during setup, including runtime socket, container user, namespace mode and volume options.
- Added separate storage modes for local files, local S3-compatible storage and external S3-compatible storage.

### Changed

- Made the main deployment profile runtime-neutral and enabled Full local data and local S3 profiles only when selected.
- Replaced separate Core and Full Compose files with one runtime-neutral Compose file controlled by deployment profile, storage mode and optional service profiles.
- Normalized upgrade handling before restart by validating compose configuration, data roots and storage mode.
- Capabilities now report configured and currently available states separately, including unavailable reasons where applicable.
- Documented the supported upgrade path around `setup.sh --quiet`, Compose rebuilds and health checks.

### Fixed

- OSM imports now restore read-only grants after refreshes so OSM-backed queries keep working after data reloads.
- Route-planning chat now uses local place search for geocoding and handles routing tool responses more reliably.
- Core installations no longer show Full-only actions as startable.
- Media and document ML jobs now require the matching deployment profile and readable storage before being marked available.
- Setup now rejects unsafe data-root paths more strictly, isolates smoke-test Compose projects and seeds bundled BRouter profiles into custom data roots.
- Setup now hardens runtime bootstrap, secure temporary file handling, generated environment updates, container socket selection and storage-mode guardrails.
- External S3-compatible storage now requires explicit S3 credentials; bundled local S3 credentials are only used for local Garage installations.
- BRouter routing now requires a successful availability probe, and BRouter proxy and assistant routing tools are disabled when routing is unavailable.

---

## 2026-05-14

### Added

- Added editable system e-mail templates in the admin settings, including subject, HTML, text, preview, test send and reset-to-default actions.
- Added a login-page flow to resend verification e-mails without requiring an active session and without revealing account existence.
- Added an encrypted outgoing e-mail outbox for authentication messages when server encryption is configured.
- Added a central import page that combines upload, review, active queue and import history in one place.
- Added direct access to Garmin, Strava and Komoot bulk imports from the central import area.

### Changed

- SMTP is considered active only when the required sender, host and port settings are complete and credential fields are consistent.
- Social login providers remain disabled when the server-side encryption key is missing.
- JWT validation reloads mutable account status from the database so disabled or deleted accounts are rejected even with otherwise valid tokens.
- BRouter integration updated from `v1.7.5` to `v1.7.8`.
- Import history, active queue and manual-review items are now separated into clearer tabs.

### Fixed

- Open registration no longer creates accounts that cannot be verified when outbound e-mail is not configured.
- E-mail outbox records no longer retain clear-text confirmation links after completion, expiry or final failure.
- Long-lived e-mail jobs retry until their message expiry time instead of failing only because a fixed retry count was reached.
- Setup can repair legacy runtime directory ownership and recreate required writable directories for rootless container operation.
- ML readiness is now distinguished from process liveness.
- LLM approvals now depend on the configured context window and require a new check or approval when that window changes.
- Legacy import links now redirect to the matching view in the central import page.
- Direct photo and gear-document uploads now appear consistently in queue and history.
- S3-compatible and local media paths now behave consistently across route photos, activity photos, the import queue, document processing and ML photo embeddings.
- Optional document processing no longer marks a completed upload as failed when only the post-processing step fails.
- Review counters refresh after assignment or rejection decisions.

---

## 2026-05-13

### Added

- Added a Go-backed chat assistant that can query activity, health, training, gear, photo, segment, geodata and route-planning tools while streaming responses.
- Added mandatory preview and confirmation for assistant write actions that modify activities, gear, health data, tags or automation rules.
- Added assistant tools for bulk tagging, gear service preparation, default gear rules, tag merges, rule backfills, activity recompute jobs and route-preview saving.
- Added assistant tools for trends, personal records, highest visited geo-features and due gear maintenance.
- Added document-text search for the assistant when document-text sharing is enabled.
- Added route-planning assistant tools for local place resolution, coverage checks, profile lookup, direct route previews and route-side point searches.
- Added goal-based route suggestions in chat when local routing and coverage data are available.
- Added role-based LLM configuration for chat, document extraction, Markdown conversion and vision tasks.
- Added durable GDPR transfer artifacts so exports remain downloadable after container restarts.
- Added multipart GDPR exports with manifest, metadata archive and multiple file archives when needed.
- Added direct import support for multipart GDPR exports.

### Changed

- Migrated the old chat assistant runtime from the Python ML container to the Go API container.
- Read-only SQL access is now governed by the same assistant role and quality checks as tool calls.
- LLM role configuration is tied more strongly to quality checks, explicit approvals and configured context windows.
- The local LLM proxy configuration is generated from checked roles and uses a pinned image.
- Document and Markdown processing now request LLM roles through the backend broker instead of loading separate models in the ML service.
- Installations are configured more explicitly for local, server and development use cases.
- Local S3-compatible storage setup can be rerun safely and is separated from external S3-compatible storage.
- Database tuning is stored in central configuration instead of being hidden inside container overrides.
- The supported device catalog is now easier to read, with recording devices, sensors, developer apps and virtual platforms separated.
- GDPR downloads now use short-lived download sessions.

### Fixed

- Assistant write actions cannot mutate data without audit and confirmation.
- Routes can only be saved from a previously generated route preview with the matching token and profile version.
- Invalid or incomplete assistant tool calls are handled more robustly.
- ML LLM tasks degrade cleanly when the broker token or approved role is unavailable.
- Chat streaming, session switching, save errors and auth-refresh errors leave fewer stale messages or misleading success states.
- Personal data questions require tool use before the model answers when suitable tools are available.
- Local geosearch is no longer treated as if an unavailable external search backend were ready.
- Routing coverage checks correctly accept saved user places.
- Chat requests now respect the configured context window.
- Mixed route, weather, destination and coverage questions keep the required tool set.
- ML services now consume S3-compatible credentials and secret-file configuration consistently.
- Fresh installations initialize the read-only database role more reliably.
- Automatically generated secrets are no longer printed in non-interactive setup.
- Server installations no longer continue with contradictory local URL or TLS settings when a real domain is missing.
- External S3-compatible storage no longer starts the local S3-compatible service automatically.
- GDPR status checks no longer show missing export files as downloadable.
- Startup checks now fail visibly when export or import artifact storage is not writable.
- Quota and storage errors no longer publish incomplete GDPR transfers.
- Abandoned multipart uploads can be retried after their receive window expires.
- Late storage errors in edit sidecars no longer hard-fail an otherwise successful import.
- Legacy import links now redirect to the matching view in the central import page.
- Direct photo and gear-document uploads now appear consistently in queue and history.
- Gear documents are converted to Markdown and structured suggestions more robustly, including scanned PDFs and PDFs with unusable raw text.
- S3-compatible and local media paths now behave consistently across route photos, activity photos, the import queue, document processing and ML photo embeddings.
- Optional document processing no longer marks a completed upload as failed when only the post-processing step fails.
- Review counters refresh after assignment or rejection decisions.

---

## 2026-05-12

### Added

- Added central admin controls for media upload size, per-user storage quota, activity-media count and route-photo count.
- Added atomic count limits for activity media and route photos across manual uploads, import-queue assignments, automatic media matching and route-photo uploads.
- Added stable quota error reporting with per-file status for batch media uploads.
- Added automatic activity titles based on sport, start and destination place and detected summits or passes.
- Added actions to recalculate automatic activity titles from the detail page and workshop reprocessing.
- Added display preferences for choosing imported titles or generated titles.
- Added an activity-list filter for open lift review cases.

### Changed

- Photo deduplication is now user-wide and more robust across image format changes.
- Duplicate uploads can add missing metadata such as capture time, GPS data and source data without overwriting existing values.
- Higher-resolution replacement photos can replace originals and derivatives without increasing parent media count.
- Activity photos are registered only once as originals in the file store.
- Account creation seeds required system tags and rules in the same transaction as the new account.
- Activity lists, map labels, photo assignment, similar routes, segment views, summit views, tag views, multisport views and fusion views now use consistent server-side title resolution.
- Lift candidates from map data now pass through the review workflow instead of being counted automatically.
- Lift review visibility now considers all open candidates.
- Per-activity lift detection mode changes are saved immediately and only start a new detection job when needed.

### Fixed

- Restore imports stop cleanly when the target instance lacks enough storage quota.
- Import-queue assignments roll back correctly when a late deduplication, metadata or replacement step fails.
- Frontend upload errors are no longer treated as successful responses.
- Failed route-photo writes now clean up files that were already stored.
- Reprocessed activities preserve manual titles more reliably.
- Lift detection from map data is more tightly constrained to reduce false lift rides.
- Lift recompute completion now reloads activity details, streams, sensors, track summary and lift candidates.

---

## 2026-05-11

### Added

- Imported activities can again receive their referenced primary gear when archive data contains a resolvable gear reference.
- Gear documents can store purchase date, amount and currency directly on the document.
- Added deeper bike gear specifications, including frame, suspension, drivetrain, brakes, wheels, tires and cockpit details.
- Added custom fields for gear components.
- Added support for mixed wheel-size setups where applicable.
- Added component presets tied to compatible gear subtypes.
- Added setup snapshots that preserve the component list at the time a setup is saved.
- Added component lifecycle states: active, stored and retired.
- Added service intervals per component.
- Added named chain-rotation groups with inline editing.
- Added cumulative descent as a gear wear metric.
- Added sport badges on gear cards.
- Added component moving between bikes with compatibility warnings.
- Added quick-selection hints for typical and unusual modifier combinations.
- Added automation rule conditions based on gear wear totals.
- Added more precise sport assignments for imported gear.
- Added lift detection, review, statistics and recomputation directly in the enrichment pipeline.
- Added lift candidate review on activity details with compact and expanded inspection modes.
- Added lifetime lift statistics and optional lift-distance subtraction from activity and gear totals.
- Added per-activity and global lift detection settings.
- Added soft rejection of lift candidates so user decisions survive reprocessing.
- Added a bulk operator command to recompute lift detection for one user or all users with lift detection enabled.

### Changed

- Document processing now distinguishes empty extraction results from processing failures.
- Gear schema errors are detected earlier during build.
- Technical freehub labels are named by standard rather than by marketing language.
- Lift detection now runs in the main backend pipeline instead of the ML service.
- Lift labels use map names when available.
- Gear statistics account for confirmed lift subtraction.

### Fixed

- Service intervals remain correct after service entries are deleted.
- Chain rotations only accept components from the same gear item.
- Document-derived gear suggestions are mapped to valid gear types and scopes before saving.
- Duplicate climbing-harness selection was removed while keeping stored values readable.
- Parallel lift recomputes for different activities of the same user no longer block each other.
- Rejected heuristic lift candidates no longer block later map-based candidates for the same time span.
- Lift rides from secondary grouped activities no longer count twice in user statistics.

---

## 2026-05-10

### Added

- Added a type filter to import history for activities, routes, workouts, monitoring data, photos, videos, documents, gear, segments, archives and unknown entries.
- Added an `In Queue` status for photo and document uploads that need manual assignment.
- Added route, gear, monitoring, photo and GDPR archive entries to import history.
- Added import-history entries for manually uploaded media and documents from the moment of upload.
- Added component-specific dialogs on the gear detail page.
- Added searchable long select lists, multi-select fields, unit-aware fields and conditional fields for gear components.
- Added detailed fork and shock specifications.
- Added drag-and-drop ordering for free components.
- Added sport suggestions when creating gear, including additional suggestions for snow, water, diving and climbing gear.
- Added unsaved-change protection in component dialogs.

### Changed

- Workouts moved out of the main navigation and are linked from the activity list header.
- The gear detail page now uses tabs for base data, geometry, sport assignment, components and wear.

### Fixed

- Monitoring FIT files imported by URL are now stored in the correct health tables.
- Import-history status filtering now rejects invalid values instead of silently returning an empty list.
- Photo and document history entries now link to the assigned activity after assignment.
- Platform photo imports without capture timestamps now remain in queue for manual assignment.
- Route and gear import entries now reliably finish with completed status.
- Monitoring FIT uploads are consistently classified by file content.

---

## 2026-05-09

### Changed

- Modifier-tag synchronization is now more robust during concurrent bulk operations, imports and rule runs.
- Added an explicit repair function for operators to resynchronize system modifier tags from activity flags.
- Reserved modifier tag names are treated as system tags.
- System tag renames are reflected immediately in affected activities.
- Renaming a system tag to another reserved name now returns a clear conflict message.
- Direct removal of a system modifier tag link updates the related activity flag as well.

### Fixed

- The electric-bike system tag is synchronized only for cycling activities.
- Changing the sport of a cycling activity no longer triggers a database error in the modifier-tag sync path.

---

## 2026-05-08

### Added

- Added new sports and subtypes across filters, rules, imports, routes, segment eligibility and saved predicates.
- Added a central modifier registry for indoor, electric, virtual and commute modifiers.
- Added quick editing for activity metadata, including title, visibility, sport tuple, modifiers, heatmap/statistics exclusions, tags, description, perceived effort and feel.
- Added generated record and best-effort groupings from the sport taxonomy.
- Added sport subtype and modifier predicates for gear defaults.
- Added metrics and warnings for unknown sport mappings delivered by import formats.
- Added an optional internal metrics listener.

### Changed

- Import sport values are normalized from source-specific aliases into canonical sport tuples and modifiers.
- Best-effort recomputation is more robust for large datasets and uses checkpoints.
- The metrics endpoint can be served through an admin-protected application path or a separate internal listener.
- Local activity file paths now include canonical sport, subtype and modifier tokens.
- Rules, routing profiles, segments and heatmaps now use combined sport predicates.
- Database container defaults are tuned for safer filesystem operation.

### Fixed

- The electric-bike tag is limited to cycling activities.
- Virtual activities remain excluded from heatmaps.
- Predicate validation rollback handles real post-update activity data more safely.

---

## 2026-05-05

### Changed

- Summit markers are now rendered consistently on the detail map and full-screen map.
- Place point markers were removed from the activity detail map because place areas are not point features.

### Fixed

- The full-screen map no longer opens as a black map while style and capability data are still loading.
- Track gaps no longer disappear incorrectly when zoom level changes.
- The summit summary updates immediately after recomputation instead of relying on stale cached data.

---

## 2026-05-04

### Changed

- Added a clearer priority tooltip in the rule editor.
- Moved remove-condition controls to the end of the condition row.
- Made action and condition sections visually consistent.
- Moved add-action controls below the action list.
- Standardized add buttons for conditions, actions, groups and branches.

---

## 2026-05-03

### Added

- Added a map layer panel that lists active overlays with checkboxes, labels and options.
- Added drag-and-drop layer ordering.
- Added separate zones for layers above and below activity tracks.
- Added arrow controls as a non-drag ordering alternative.
- Added expandable coverage overlay groups.
- Added opacity sliders for custom tile layers.
- Added a direct link from the layer panel to custom tile-layer settings.

### Changed

- Replaced the old overlay dropdown with the new layer panel.
- Removed the separate map-layer section from the filter sidebar.

### Fixed

- Custom layer opacity sliders no longer jump back after saving.
- Traffic overlay order is preserved when toggled.
- Custom tile layers can be moved between both layer zones.

---

## 2026-05-02

### Added

- Added route edit conflict detection when the same route is saved from multiple browser tabs.
- Added a full-screen year-comparison chart with multi-year overlays and a comparison table.
- Added rolling-window mode for year comparison charts.
- Added an API endpoint for rolling year-progress data.
- Added active-filter badges on hybrid analytics charts.

### Changed

- The global analytics filter now applies only to activity-based charts.
- Career statistics now have their own sport filter.
- Health charts now have independent time-range selectors.

### Fixed

- Background rule errors now include the affected activity ID.
- Health endpoints no longer appear to apply global filters that they intentionally ignore.

---

## 2026-05-01

### Added

- Added a readiness card on the dashboard and insights page with a 0–100 score and five readiness levels.
- Added a readiness scale visualization and up to eight contributing factors with status and source labels.
- Added a source switch between wearable score and internal calculation when both are available.
- Added recommendation text and clear empty states for missing readiness factors.

### Changed

- Readiness, recovery and related scores now use a single calculation source across dashboard and insights views.
- Readiness cache is reused while a background refresh runs and is invalidated after relevant health or activity imports.
- The insights page can show readiness even when other insight sections are empty.
- Health imports now trigger readiness recomputation.

### Fixed

- Degraded-response handling now works with Unicode whitespace in JSON.
- Reconcile publishing is rate-limited and avoids blocking the database connection while waiting.
- Recompute triggers are capped across all paths.
- Duplicate readiness display in the insights tab was removed.

### Security

- Readiness preferences are restricted to the owning user account.

---

## 2026-04-30

### Changed

- Activities, planned routes, recording groups and import/fusion paths now store sport variants as a canonical sport plus optional subtype and modifiers.
- Legacy and source-specific sport values are normalized before runtime storage.
- Routing profiles now match by predicates and modifier filters.
- Segment sport filters now use allowed predicates instead of separate legacy sport arrays.
- Planned routes and route suggestions preserve sport tuple and modifiers through create, update, import, filter and profile matching.
- Activity lists, maps, analytics, records and search can combine parent sports, exact subtypes and modifiers.
- Automation rules now write canonical sport tuples and modifiers.
- Fusion and recording groups compare and preserve sport, subtype and modifier information together.
- Assistant tool vocabulary now uses the same sport tuple and modifier rules as the API.

---

## 2026-04-29

### Added

- Added an AGPL-3.0 license file.
- Added a private vulnerability reporting process.
- Added automated secret scanning, static security analysis, supply-chain checks and dependency update automation.
- Added container health checks for API, ML and frontend services.
- Added standardized image metadata labels.

### Changed

- ML builds are now reproducible by pinning the external source dependency to a specific revision.

---

## 2026-04-27

### Added

- Added route elevation profiles that cover the full drawn route including approach and departure segments.
- Added visible gaps in elevation profiles where terrain data is missing.
- Added a one-time session warning when planning in an area without elevation data.
- Added admin map-layer management for ordering, visibility, default zoom, default opacity and custom tile sources.
- Added per-user private custom tile layers.
- Added disabled-but-visible map layers with explanatory tooltips when a tile source is incomplete.

### Changed

- Elevation profile updates now handle concurrent waypoint edits more accurately.
- Older routes are completed with missing profile sections when loaded, but changes are persisted only after manual save.
- Map layer selection now reads from database configuration instead of a hardcoded list.
- Technical vector and composite layers are hidden from the general layer switcher.

### Fixed

- Elevation-data drift is reported in health status.

---

## 2026-04-24

### Added

- Added admin controls for user library folder naming and file-schema templates.
- Added default schema templates for activities, media, documents, health files and routes.
- Added per-user library folder changes and bulk rename jobs.
- Added segment editor improvements including native colored selects, edit-mode segment type selection, multi-filter difficulty matching and multi-visibility filtering.
- Added segment bulk selection, individual GPX/GeoJSON export, ZIP bulk export and bulk delete with confirmation.
- Added URL-persistent segment filters and recovery controls for filter errors.
- Added water-sport and lift segment types in the editor.
- Added leaderboards, range brush zoom, elevation-to-map hover sync, safer range sliders, save-to-detail navigation and reset options for segment editing.
- Added collapsible planner and elevation panels on the map page.
- Added responsive map controls for narrower screens.
- Added persistent full-resolution route elevation profiles and enrichment data.
- Added lazy elevation-profile fill for older routes.
- Added elevation profiles in route suggestions.

### Changed

- User settings now show the effective library schema and mark admin-locked fields as read-only.
- Segment surface selection is now available for more segment types where relevant.
- Segment difficulty badges were redesigned with separate SAC and MTB information.
- Segment color synchronization now follows the selected difficulty values.
- Route loading no longer recalculates unnecessarily when stored geometry and profile data are available.
- Switching routing engines keeps a loaded route visible until the target engine needs recomputation.
- Saving a loaded route now updates the existing route instead of opening a create dialog.
- Route GPX export defensively applies the stored profile to elevation output.

### Fixed

- Segment edit buttons are hidden from users who cannot edit the segment.
- Segment update authorization now returns a clearer forbidden response for non-owners.
- Problem-response bodies are shown in frontend error messages.
- Lift filtering no longer hides all non-lift segments.
- Segment preview geometry is unwrapped correctly.
- Light-mode contrast and padding issues in segment editor controls were fixed.
- Segment detail maps now request geometry explicitly.
- Route editor navigation from the routes page now opens the correct planner view.
- Loaded routes now fit the map to their route bounds.
- Multi-tile elevation spikes in route profiles were removed by deriving elevation from the stored profile.
- BRouter elevation values are preserved in route profiles when available.
- Suggestions and loaded routes now populate the effective profile correctly.

---

## 2026-04-21

### Added

- Added route difficulty badges for SAC, MTB and via ferrata levels.
- Added a safety indicator for unscaled via ferrata sections.
- Added interactive route map overlays for difficulty levels, forest sections and protected-area sections.
- Added route enrichment parity between direct route calculation and route suggestions.
- Added summit proximity lookup for routes and activity tracks.
- Added parsing for comma-decimal via ferrata scale values.
- Added a user setting for summit detection radius.
- Added profile controls for maximum via ferrata level and whether unscaled via ferrata sections are allowed.

### Changed

- Forest and protected-area percentages are normalized consistently across route endpoints.
- Descriptor highlighting now works for planned routes, loaded routes and route suggestions.

---

## 2026-04-20

### Added

- Added a full-screen segment editor with track, draw and lap modes.
- Added segment reverse support with rank recomputation and safe concurrency behavior.
- Added OSM-based segment metadata suggestions for surface, difficulty and color.
- Added winter-sport segment support.
- Added bidirectional matching controls for segments.
- Added map preview, direction arrows, style switching, heatmap overlay and dirty-close protection in the segment editor.
- Added a group-title backlink on the fusion editor page.
- Added timestamp inputs next to trim and split sliders in the activity editor.
- Added active-filter display and reset controls in the summit collection.

### Changed

- Segment editor controls now use a consistent orange accent and standardized pill components.
- SAC and MTB difficulty colors use a consistent three-color scheme.

---

## 2026-04-19

### Added

- Added metric-based activity route coloring on detail and full-screen maps for heart rate, pace/speed, elevation, cadence, power and temperature.
- Added zone coloring for heart rate, power and speed when zones are configured.
- Added legends for linear and zone-based route coloring.
- Added GPS teleport filtering so implausible jumps are rendered as gaps.
- Added clearer route-suggestion startpoint handling based on an existing start waypoint when present.
- Added route outdoor descriptors for technical MTB, alpine hiking, exposed sections and low visibility.
- Added interactive descriptor chips and descriptor map highlights in route suggestions.
- Added trail-visibility profile controls for routing.
- Added more route segment metadata fields for outdoor descriptors.
- Added warning output for non-fatal fusion merge issues.
- Added post-merge elevation recomputation from merged streams.
- Added per-activity segment-effort recomputation after fusion.
- Added group-context handling for selected pair or full-group fusion.
- Added an endpoint to add recordings to an existing recording group.

### Changed

- The old route-variant pipeline was removed in favor of the newer route suggestion flow.
- Route suggestion acceptance now loads the full suggested geometry instead of recalculating from only the visible waypoints.
- Fusion merge now uses the canonical endpoint and returns immediate conflict responses when concurrent merge locks cannot be acquired.
- Fusion map and chart handling now better support stationary recordings, GPS gaps and duplicate tooltip values.
- Recording-group updates are now integrated into the merge transaction.
- Segment recompute during fusion is now linear in the number of matches instead of repeatedly scanning per segment.
- Warning toasts become sticky for long or numerous fusion warnings.

### Fixed

- Route saving now includes geometry and no longer fails with an empty geometry payload.
- Descriptor chips no longer overlap route summary columns.
- Full-screen map photo thumbnails now use the correct thumbnail field.
- Fusion cleanup now uses the stored activity GPS flag instead of deriving it from an unbuilt track.
- Fusion merge and revert now invalidate all affected frontend caches.
- Descriptor highlight state and expanded route state no longer desynchronize after suggestion changes.
- Zero-distance edges no longer pollute outdoor descriptor aggregation.
- Group membership no longer leaves zombie members after fusion.
- Concurrent group-member updates and fusion merges now return a conflict instead of corrupting state.
- Background maintenance work now continues safely after a request disconnects.

### Removed

- Removed the old route variant API fields and legacy route variant selection code.
- Removed unused fusion field-copy code paths.

---

## 2026-04-16

### Added

- Added a dedicated fusion membership table as the authoritative source for fusion relationships.
- Added dismissals for fusion candidates.
- Added source tracking for photos moved through fusion.
- Added insert-based stream merge support for compressed time-series data.
- Added advisory locking and conflict preflight checks for parallel merges.
- Added sidecar metadata for fusion and manual-entry activities.
- Added fusion membership export support.

### Changed

- Fusion source activities are now hidden through parent linkage instead of being deleted or overwritten.
- Fusion field copying and stream merging are clearly separated in API and UI terminology.

---

## 2026-04-15

### Added

- Added opt-in external reverse geocoding for activities without a resolved start city, including caching, rate limits and negative caching.
- Added a backfill UI for missing start-city data.
- Added settings for provider selection, URL override and timeout.
- Added more robust restoration of map-data schema columns after data imports.

### Changed

- Automation rules with empty condition sets now evaluate to false by default.
- Reverse geocoding now uses user settings and can preload settings for large backfills.
- Geocoding backfill errors now return structured error codes.

### Fixed

- Boundary-crossing detection now triggers only for real boundary crossings.
- Interval training tags were unified to one spelling.
- Speed-based interval fallback was removed to reduce false positives.
- Recompute paths no longer overwrite accumulated manual, automatic or rule-generated tags with only source-file tags.
- Start-city lookup no longer depends on a missing map-data column.
- Weather tag synchronization no longer rebuilds the full activity tag array.

### Security

- External reverse geocoding validates schemes and blocks private, loopback, link-local and metadata-network addresses.

---

## 2026-04-14

### Added

- Added an `IMPORT_ALLOWED_DIRS` configuration to explicitly allow path-based bulk imports outside the safe default directory.
- Added a capability flag that shows whether path-based imports are enabled.
- Added admin user promotion, demotion and deletion actions with confirmations.
- Added per-user hotfolder toggles when hotfolders are admin-controlled.
- Added soft-deleted user badges in the storage dashboard.
- Added readable audit labels for user administration events.

### Changed

- Import wizard path inputs are shown only when path imports are explicitly enabled.
- Admin system settings use consistent cached queries.
- Compression jobs and TimescaleDB state changes now run only when the setting actually changes.
- Storage limit fields now use API-provided defaults as placeholders.

### Fixed

- Path-based imports now resolve symlinks before allowlist checks.
- Account and user deletion dialogs no longer submit through Enter-key bypasses.
- Last-admin checks are transactionally protected.
- Database errors in last-admin checks are no longer swallowed.
- Hotfolder toggles verify that the target user exists and is not deleted.
- Storage limits are positioned in the correct admin section.
- Empty limit values now show a validation error instead of failing silently.
- Library sync cannot be enabled for S3-compatible storage when unsupported.
- Storage dashboards count active and archived files.
- User cleanup now includes pending e-mail and e-mail token data.

---

## 2026-04-13

### Added

- Added runtime server settings for e-mail, registration, social login, limits, hotfolders, library sync and database compression.
- Added a System tab in admin settings for e-mail, registration, social login, filesystem and database settings.
- Added a rewritten setup flow with environment detection, database tuning, storage options, SMTP configuration, registration mode, compression and bootstrap admin support.
- Added file-based secret support for sensitive environment variables.
- Added an admin test-e-mail endpoint with rate limiting.
- Added a redesigned GDPR import engine that imports files first, resolves JSON references later and reports step progress.
- Added a GDPR import CLI and API with status polling and frontend upload flow.
- Added file import through the unified file store during GDPR imports.
- Added post-import recomputation jobs for enrichment, heatmap, performance and segment matching.
- Added English and German support for Strava archive imports.
- Added an S3-compatible connectivity guard that prevents startup when configured storage is unreachable.

### Changed

- SMTP sending now supports implicit TLS, mandatory STARTTLS by default and relay servers without authentication.
- Mail sending reads current runtime settings instead of fixed startup configuration.
- Frontend URL and application URL handling were unified.
- Import conflict handling now treats missing file references as null instead of inventing phantom IDs.
- GDPR import progress is now step-based.
- System tags are correctly categorized as system tags instead of custom tags.
- Strava imports parse localized headers, date formats, number formats, weight units, gear sport lists and privacy-zone radius units.

### Fixed

- SMTP on implicit TLS ports now works correctly.
- Runtime settings are saved transactionally.
- Secure-cookie detection now uses the current application URL.
- File attachment references no longer create phantom file IDs.
- Edit sidecar import no longer double-remaps parent IDs.
- JSON reference remapping no longer drops keys when an ID mapping is missing.
- Monitoring and health imports preserve negative elevation and zero-degree temperature values.
- Weight parsing accepts pounds in more input forms.
- S3-compatible bucket checks cover all configured buckets.

---

## 2026-04-12

### Added

- Added a round-trip route quality gate using reachable-area analysis, route variants and quality classification.
- Added route suggestion metrics and UI labels for loop quality and target fit.
- Added outdoor-aware one-way handling for routing profiles.
- Added authentication proxying for local vector tiles so user-specific map data is served only through the API.
- Added automatic TSS recomputation when training zones change.

### Changed

- Round-trip suggestions now use a newer deterministic route candidate flow.
- One-way handling now distinguishes trail-like and road-like contexts by routing profile.

### Fixed

- User heatmap data is no longer exposed through unauthenticated tile access.
- Current-session detection now uses the configured refresh cookie name.
- Route suggestion error handling returns a failure instead of an empty success response when route building fails.
- Route suggestion colors now cover the supported maximum number of variants.

### Security

- Local tile requests now require authentication and enforce the current user context server-side.

---

## 2026-04-11

### Added

- Added centralized account-settings strings to prepare for localization.

### Changed

- Activity charts now use downsampling and progressive rendering more carefully for dense time-series data.
- Developer field charts use a more memory-efficient data structure.

### Fixed

- GDPR export now checks schema version, ZIP writer errors and manifest metadata more robustly.
- GDPR purge audit logging now treats missing audit entries as an error path.
- GDPR purge uses per-user locking and retry handling for serialization failures.
- Password-change validation no longer hides database failures as unauthorized responses.
- First-time setup is no longer blocked by invite-only registration mode when no user exists.

---

## 2026-04-06

### Added

- Added API and ML liveness, readiness and health endpoints with deployment-friendly semantics.
- Added JSON logging configuration for the ML service.
- Added health checks for all major containers.
- Added a production frontend container with SPA routing, asset caching and compression.
- Added optional service profiles for S3-compatible storage, ML and LLM services.
- Added safe bootstrap-admin creation for fresh installations.
- Added a setup flow for prerequisites, domains, certificates, storage backend, feature profiles and port conflicts.
- Added documentation and sample environment configuration for self-hosting.
- Added TCX lap extension parsing, maximum speed import and additional device/developer app recognition.
- Added developer-field columns for body temperature, radar, running smoothness and glucose streams.
- Added blood-pressure and weight/body-composition manual entry flows.
- Added effort percentile badges, personal-record progression timelines, six-way activity comparison and training-zone editing.
- Added automatic best-effort computation on import and a full-user recomputation endpoint.
- Added suspicious-power advisories for best efforts.
- Added a complete GDPR export engine with background jobs, status polling, streamed download, manifest, schema metadata and rate limiting.

### Changed

- Hardcoded data directories were replaced by configuration fields.
- The API is exposed through the frontend/proxy path by default.
- Elevation and routing data volumes are writable where downloads require it.
- Container images are pinned instead of using floating tags.
- Health and chart queries are parallelized or batched where safe.
- Activity comparison now avoids double downsampling.

### Fixed

- Local HTTP setup no longer enters an HTTPS redirect loop.
- Rootless container volume ownership works more reliably.
- Bootstrap and health endpoints are safe with missing database handles.
- ML health checks no longer fail because the health-check binary is missing.
- Required secrets are generated reliably in quiet setup mode.
- Routing profile uploads now write to the configured profile directory.
- Frontend waits more reliably for API readiness on startup.
- Personal-record tie handling is stable.
- Repeated developer-field backfills no longer null populated activity fields.
- Glucose chart thresholds are separated for exercise and fasting contexts.
- Running-dynamics charts appear only for activities with running-specific data.
- Mobile health timestamps with time-of-day no longer overwrite same-day measurements.
- Background backfills use graceful shutdown and per-user timeouts.
- Mobile health data has explicit source priority.
- GDPR deletion now covers renamed and previously missing health tables.
- GDPR export no longer marks a purge job as complete outside the deleting transaction.

### Security

- API and ML containers now run as non-root users.
- Local S3-compatible secrets are generated from templates instead of committed defaults.
- GDPR export and cleanup validate temporary file paths.
- User-facing GDPR export errors are sanitized.

---

## 2026-04-05

### Added

- Added a unified geofence condition with start, end and track checks, circle or polygon shapes and inside/outside modes.
- Added track-crossing geofence checks for automation rules.
- Added a commute rule flow from a manual commute tag and from privacy zones.
- Added dwell checks for long stationary stays inside a geofence.
- Added route profile defaults from JSON configuration and configurable road/path cost factors.

### Changed

- Geofence conditions are now normal rule conditions instead of a special rule-editor section.
- Explicit commute rules replaced the previous ML-based commute clustering approach.
- Routing profile import accepts both current and older profile formats.

### Fixed

- Commute analytics filters use the current system-tag fields.
- Dry-run evaluation guards now also apply to fallback branches.
- Routing profile UI sliders and defaults match backend ranges.

---

## 2026-04-04

### Added

- Added an automatically generated supported-device reference.
- Added personal-record detection for distance, elevation gain, power and superlative efforts.
- Added near-miss detection and personal-record badges on activity details.
- Added a personal-records page with sport and year filters.
- Added top-three segment personal rankings.
- Added running form analysis with duty factor, vertical ratio, ground-contact balance and fatigue index.
- Added sport-specific and combined performance-management charts.
- Added incremental map-data update endpoints and scheduled updates.
- Added multi-route comparison, landscape preferences, surface preference and custom routing profiles.
- Added route coverage scoring and user-managed routing profiles.
- Added recognized sensor sources from developer-field apps.

### Changed

- Best-effort and running-form recomputation can be selected in reprocessing scopes.
- Background jobs are guarded per user rather than globally where appropriate.
- Stale running jobs are cleaned up on startup.
- Route suggestion hover and keyboard behavior were improved.

### Fixed

- Soft-delete cleanup, fusion restore and sport edit safety were improved.
- Advisory locks now pin database connections correctly.
- Map-data update schedules use the correct update timestamps.
- Power best-efforts now use time-weighted averages.
- Running-form and performance charts received formatting and baseline fixes.
- Route profile validation and preference clamping were hardened.
- Industrial-noise coverage no longer includes solar parks.

---

## 2026-04-03

### Added

- Added a generic developer-field chart for activity details.
- Added a sticky chart controls header.
- Added a draggable chart legend chip bar.

### Fixed

- Device-name resolution was improved for recording devices, watches, sensors and developer-field sources.
- Grouped recordings now show the correct recording device name.
- Activity chart tooltips, units, ordering, map hover marker behavior and power-phase styling were corrected.

---

## 2026-04-02

### Added

- Added route planner layout improvements, surface display and drag-to-insert waypoints.
- Added hover-to-highlight for route surfaces on the map.
- Added manual input for numeric routing sliders.
- Added workout-linked activity counts.

### Fixed

- Route planner issues around disconnected road profiles, shrinking waypoint lists and filter panels were fixed.
- Activity comparison tooltip alignment and route planner navigation issues were corrected.

---

## 2026-04-01

### Added

- Added ELSE and ELSE-IF branches to automation rules.
- Added trigger-event filtering, stop-on-match behavior and retroactive application controls.
- Added webhook and in-app notification rule actions.
- Added dry-run previews with activity metadata.
- Added recording group device tabs on activity details.
- Added expanded fusion badges with member rows, device details and elevation information.

### Changed

- Fusion editor layout now places title and tags above the field table.
- Map layout, planner panel scrolling and overlay alignment were improved.

### Fixed

- Automation branch evaluation now runs in the correct order.
- Tag addition skips deleted tags.
- Rule branch display, loading, formatting and card display were corrected.
- Webhook execution drains responses and uses safer network checks.
- Activity comparison now aligns all sources to a shared timeline.
- Routing now follows road geometry when retracking partial edges.
- Map full-screen overlay, primary-source updates and source-card interactions were fixed.

---

## 2026-03-31

### Added

- Added a broader automation-rule system with trigger filtering, stop-on-match, retroactive flags, post-enrichment hooks, recommended rules, dry-run previews and many new conditions and actions.
- Added recording groups with automatic group detection, group pages, fusion queue integration and secondary-activity filtering.
- Added expanded FIT file support for developer fields, dive data, health context, workouts, power phase, GPS accuracy and heading.
- Added developer-field charts and running power support from compatible foot pods.
- Added dive analytics, gas tables, decompression overlays and oxygen-pressure charts.
- Added route edge projection, draggable route control points and off-road segment editing on the main map.
- Added segment export, segment bulk export, segment bulk delete and select-all operations.
- Added barometric pressure charts and a map view-mode off switch.

### Changed

- Fusion merge copies more activity metadata and stream columns.
- Recording-group pagination is now applied to groups rather than joined rows.
- Dive charts and route-planner interactions were improved.

### Fixed

- Fusion badge updates, group integration and secondary-activity filtering were corrected.
- Routing error handling, approach layers, elevation prefixing and route fallback behavior were improved.
- Dive chart lifecycle, multi-axis charting and gas data handling were fixed.
- Capabilities queries were optimized.
- Weather data is now loaded for rule evaluation.
- Developer-field chart rendering, negative-value handling and dark-mode tooltips were corrected.
- Map and route planner bugs around dragging, double clicks and layer guards were fixed.

### Removed

- Removed the separate off-road editor component because editing moved to the main map.
- Removed straight-line fallback routing in favor of structured route errors.

---

## 2026-03-30

### Added

- Added indoor activity taxonomy and an indoor system tag.
- Added an indoor badge in the activity list.
- Added a location-presence filter.
- Added a map-based location assignment section for activities without GPS.
- Added a unified speed/pace chart with sport-specific defaults.
- Added chart image export.
- Added segment export and bulk operations.
- Added a sport taxonomy label system and typed API interfaces for many activity detail data types.
- Added chart tabs, picture-in-picture map, gradient coloring in elevation profiles and sport-specific statistic components.

### Changed

- Indoor assignment and map filter labels now refer to location assignment rather than only indoor pins.
- Speed/pace controls were moved into chart headers.
- Activity detail pages were split into focused components.
- Activity files use better centralized download and authorization helpers.

### Fixed

- Legacy fixed-offset timezones no longer crash date formatting.
- Activity calendar and dashboard date handling now normalizes fixed-offset timezones.
- Existing offset timezones can be upgraded to IANA names when GPS coordinates are available.
- Unwanted chart brush toolbar icons were removed.
- Pace and speed labels were corrected.
- Routing, capability checks, location picker, route planner and segment export issues were fixed.

### Removed

- Removed older separate speed/pace and GAP chart components after consolidation.

---

## 2026-03-29

### Added

- Added a background job for computing fusion candidates and caching the result.
- Added proactive fusion-cache computation after imports.
- Added cross-tab event sharing so only one browser tab holds a live job stream.
- Added cross-tab authentication synchronization.
- Added server-side limits and periodic revalidation for live job streams.
- Added automatic activity timezone detection from GPS with fallbacks.
- Added VO2max, GPS heading, dive settings, dive gases, sport/activity messages, zone times, device software version and timezone-offset parsing from activity files.
- Added many additional device and manufacturer mappings.
- Added an admin endpoint to backfill timezones.
- Added a rebuilt training-load calculation pipeline with multiple TSS methods, thresholds, TRIMP, rolling ACWR and projections.

### Changed

- Fusion queue responses now include both items and a computing flag.
- Job and notification polling now use the live stream as the primary source with a slower safety net.
- Background job errors are truncated for display.
- Job completion labels are centralized.

### Fixed

- TCX imports now tolerate floating values in integer-like fields.
- Device serial-number matching no longer collides across manufacturers.
- Development manufacturer and product resolution was improved.
- Dive summary fields and TCX lap parsing were fixed.
- Missing activity session fields are now extracted.
- Normalized power calculation no longer treats missing power samples as zero.
- Several health, timezone, virtual-activity and recompute idempotency issues were corrected.
- Multi-tab connection exhaustion and live-stream goroutine leaks were fixed.

---

## 2026-03-28

### Added

- Added private and public segment visibility with owner and admin access rules.
- Added segment publish flow with all-user recompute.
- Added segment delete authorization and admin audit logging.
- Added segment leaderboard privacy controls that hide sensitive health metrics for other users.
- Added coverage visualization layers for nature, water, land use, protected areas, urbanization, traffic, noise and quietness.
- Added persistent binary coverage grids and a coverage tile API.
- Added coverage build actions in admin settings and import pipelines.

### Changed

- Coverage reading now uses disk-backed grid files with an LRU cache.
- Large source files and map pages were split into domain-focused components.
- Segment ranking counts public efforts for global ranking and all efforts for personal ranking.

### Fixed

- Coverage tile seams, boundary tile discovery and zoom-dependent gaps were fixed.
- Activity file format detection now respects FIT header length.
- TCX uploads get the correct content type.
- Wrapped database no-row errors are handled consistently.
- Auth responses consistently include no-cache headers.
- Empty or unused queue paths were removed.

### Security

- Segment visibility is enforced with allowlisted public checks.
- Cross-user leaderboard health metrics are removed.
- Segment deletion uses sentinel errors to avoid existence leaks.

---

## 2026-03-27

### Added

- Added parsing for additional FIT file types including monitoring, sleep, oxygen saturation, sport zones, workouts, courses, weight, segments and training readiness.
- Added high-resolution health monitoring storage and daily aggregates.
- Added training zones with source-priority upsert behavior.
- Added health monitoring endpoints with auto-bucketing.
- Added routing of all FIT uploads through a shared file-type dispatcher.
- Added file storage for monitoring FIT files.
- Added a tag-cover retrieval endpoint.
- Added metadata sidecars for activity files in local and S3-compatible storage.
- Added companion file API support in the file store.
- Added an admin sidecar backfill endpoint and UI action.
- Added restore support for S3-compatible backends.
- Added sidecar tracking in the file registry.

### Changed

- Health sleep, weight and oxygen-saturation data now store more fields.
- Health upserts now consistently use source priority.
- Bulk imports temporarily adjust compression policy for better performance.

### Fixed

- Tag cover uploads now persist the cover file and response data correctly.
- Tag cover thumbnails are generated again.
- Gear document upload now classifies image documents correctly.
- Photo thumbnails and previews can be recomputed reliably.
- S3-compatible photo serving uses the API proxy instead of browser redirects to internal hostnames.
- Storage migration is idempotent after duplicate registry entries.
- Activity updates and recomputes write metadata sidecars correctly.
- Companion sidecars are deleted correctly in S3-compatible storage.

---

## 2026-03-26

### Added

- Added a unified file store with per-file backend dispatch for local and S3-compatible storage.
- Added an offline storage migration CLI with checksum verification.
- Added a maintenance-mode guard during storage migration.
- Added thumbnail and preview files as registry entries.
- Added batch thumbnail lookup for photo feeds.
- Added an admin thumbnail backfill endpoint.
- Added a storage reconciler for dangling records, orphan objects and integrity warnings.
- Added file registry and generic attachments for stored files.
- Added per-file-type schema settings and library folder settings.
- Added a local library watcher with real-time and periodic scanning.
- Added an admin storage dashboard and reconciliation trigger.
- Added restore support from filesystem backups.

### Changed

- All file I/O now flows through the unified file store.
- Derivative files are excluded from quota and ZIP export where appropriate.
- Import queues use generic backend and path columns.
- Garmin and Strava importers now enforce safer path and size handling.
- Profiling endpoints are disabled by default and require explicit enablement.
- Local storage is the default backend.

### Fixed

- Local writes no longer exhaust readers after filename collisions.
- S3-compatible uploads no longer double-buffer large files.
- Partial local writes are cleaned up on disk-full errors.
- ZIP exports normalize paths to prevent path traversal.
- Vector-literal validation was hardened.
- Storage reconciliation supports S3-compatible storage in parallel.
- GDPR purge now deletes S3-compatible media and edit sidecars through the unified file store.

### Security

- URL imports block private and loopback addresses.
- Gear and component deletion check ownership.
- Library watcher paths validate user identifiers.
- Uploads fail when no storage backend is configured instead of silently losing files.

---

## 2026-03-25

### Added

- Added configurable highway and path cost factors for routing profiles.
- Added an endpoint that exposes full routing profile defaults.
- Added a highway-cost editor in the profile UI.
- Added custom JWT claims for admin, e-mail verification and forced password-change state.
- Added password-change enforcement middleware.
- Added resend-verification, change-email, confirm-email-change, account-delete, cancel-deletion and data-export account endpoints.
- Added a richer current-user endpoint and profile update endpoint.
- Added session IP address and current-session markers.
- Added admin user creation, password reset, restoration and audit-log views.
- Added a GDPR purge background job for expired soft-deleted users.
- Added account, admin user and audit-log UI pages.
- Added email verification banners and guards for admin and forced password change.

### Changed

- Admin checks use JWT claims and refresh after role changes.
- OAuth-only users can set a password without an existing password.
- Unverified users can log in and are guided by the frontend banner.
- Username validation is shared across registration, profile editing and admin user creation.

### Fixed

- A routing parent-chain issue for gravel routes was fixed.
- Seed handling for long routes was widened.
- A legacy external-auth identifier column was removed.

---

## 2026-03-24

### Added

- Added native e-mail/password authentication with secure password hashing, refresh-token rotation and audit logging.
- Added social login support, mobile PKCE authentication, registration modes, e-mail verification and password reset.
- Added session listing and revocation for users and admins.
- Added rate limiting, trusted-proxy handling, security headers and fetch-metadata CSRF protection.
- Added virtual activity detection and a virtual activity flag.
- Added heatmap and statistics exclusion flags per activity.
- Added virtual activity list filters, analytics filters and visual badges.
- Added virtual activity support for manual activities.
- Added landmark selection improvements for long-distance routing.
- Added additional elevation data for more regions.

### Changed

- Authentication now uses native tokens and no longer requires the previous external authentication stack.
- Virtual activities are represented as an orthogonal flag rather than as separate sport types.
- Heatmap inserts and statistics queries now respect exclusion flags.
- Hiking routing now prefers trails and tracks more strongly.

### Fixed

- Password hashing, OAuth state validation, request size limits, e-mail normalization and rate limiting were hardened.
- Elevation-related route seed selection now handles mountain crossings more reliably.

### Removed

- Removed the previous external authentication containers and related OIDC handling.

---

## 2026-03-23

### Added

- Added expanded FIT activity support with many additional fields, stream columns and detail tables.
- Added activity events, sets, jumps, HR data, segment laps, workouts and developer field definitions.
- Added routing cache budget controls and runtime configuration.
- Added map pass and peak landmark selection for routing.
- Added asymmetric bidirectional search and valley segmentation for routing.
- Added manual activities with source JSON storage and sidecar metadata.
- Added manual-entry tagging and support for perceived effort and feel.
- Added reprocessing for manual and CSV-only imported activities.
- Added shared HTTP utilities, import utilities, chart hooks, modal components and geo utilities.

### Changed

- Tile memory estimation now excludes memory-mapped sections.
- Manual activities and activity edits refresh sidecar metadata.
- Health listing handlers and background job guards were consolidated.

### Fixed

- Power phase scaling was corrected.
- Segment lap inserts no longer use the wrong placeholder count.
- Workout hash conflicts and legacy temperature field types were fixed.
- Multisport parent source-file references are stored correctly.
- Recompute jobs now limit work to the selected activity data areas.
- Ownership checks were added to gear and lift candidate handlers.
- Duration formatting defaults were restored after refactoring.

---

## 2026-03-22

### Added

- Added tile-cache monitoring counters in routing status.
- Added a border-node lookup cache for cross-tile routing.
- Added a full-screen summit collection map with filters, linked activity lists and mobile-friendly layout.
- Added a summit collection route and a link from the map filters panel.
- Added summit activity lookup endpoints.

### Changed

- Long-route expansion limits were increased.
- Neighbor tile lookup was optimized to avoid heap allocations.
- The public naming was standardized from peak collection to summit collection across UI and API.
- Summit filters now use URL state.

### Fixed

- Long-route timeouts caused by tile-cache thrashing were resolved by increasing cache capacity.
- A graph section alignment issue that forced heap copies was fixed for future tile rebuilds.

---

## 2026-03-21

### Added

- Added interval detection from split data.
- Added unified elevation filtering and sport-adaptive ascent classification.
- Added POI-first stop detection and hybrid settlement detection.
- Added a re-enrichment endpoint with module selection and background job integration.
- Added more accurate surface and way-type enrichment using nearest-way and graph matching.
- Added separate surface and way-type bars in the frontend.
- Added landmark-based routing acceleration, dynamic cost estimates and coverage query support for the assistant.
- Added a landmark-only rebuild endpoint.
- Added DEM download support for configured regions.
- Added an analytics settings tab with consolidated enrichment toggles.
- Added a shared geo package.

### Changed

- Surface grouping and colors were simplified for readability.
- Map-data category naming was standardized from peak to summit.
- Recompute now runs asynchronously with background job status.
- Coverage values are no longer written back to map-data tables unless explicitly enabled.
- Several hot paths now use batch inserts and parallel post-processing.

### Fixed

- Summit display and enrichment counters now use the correct summit category.
- Ascent detection no longer creates excessive micro-climbs.
- Stop detection ignores unnamed generic pauses and requires longer dwell time for gastronomy stops.
- Waterway and boundary enrichment was corrected.
- Device data repair fixed shadow devices and stale device-type arrays.
- Coverage query and route membership endpoints were corrected.
- Heatmap rebuild now includes point data.

---

## 2026-03-20

### Added

- Added landmark-based route acceleration and GraphReader-based isochrones.
- Added route suggestions using the tiled graph reader.
- Added a coverage query endpoint and assistant tool.
- Added more landmark POI types.

### Changed

- pgRouting and the monolithic graph path were removed in favor of the tiled graph router.
- Enrichment settings were consolidated into a single analytics settings tab.
- Tag contrast now uses a shared luminance-based utility.

### Fixed

- Geometry handling, surface rounding and route-membership enrichment were corrected.
- Settings UI action buttons were standardized.

### Removed

- Removed the pgRouting dependency and old monolithic graph tables.

---

## 2026-03-19

### Added

- Added a device and sensor registry with automatic recognition from activity files.
- Added device and sensor APIs, rules conditions, activity filters and frontend device pages.
- Added a shared geo package for distance, Hausdorff, simplification and geometry encoding.
- Added a tiled graph reader and unified router.
- Added a traffic coverage layer.
- Added in-memory segment matching and a parallel recompute worker pool.

### Fixed

- Track geometry insertion now uses binary geometry input.
- Routing regressions around turn costs, cost margins and long hilly routes were fixed.
- Coverage cache access is now concurrency-safe.
- Device deduplication and product-name update behavior were corrected.
- Background jobs now expose error text.

### Performance

- Stream insertion, heatmap insertion, post-processing, stream enrichment and recompute paths were batched or parallelized.
- Rule evaluation avoids unnecessary string formatting in hot paths.
- Database pool size was increased for parallel workloads.

---

## 2026-03-18

### Added

- Added in-memory coverage rasterization for forest, land use, water, protected areas, urbanization, river proximity and noise.
- Added configurable noise sources.
- Added an OSM pipeline management UI with separate panels for enrichment, tile building, local routing-data build and BRouter segment download.
- Added opt-in post-import pipeline steps.
- Added chunked enrichment with resume support and real progress reporting.
- Added BRouter segment download support.
- Added graceful shutdown handling for background jobs.
- Added resilient multi-region import behavior with partial-failure notifications.

### Changed

- Coverage and enrichment pipelines were moved away from memory-heavy SQL temp-table workflows.
- Routing engine naming was standardized in the UI.

### Fixed

- BRouter compatibility, coordinate ordering, download validation and shared-volume labels were fixed.
- Elevation import generation works with newer database versions.
- Stale import regions are reset on startup.
- Coverage traffic-class SQL and import cleanup behavior were corrected.

---

## 2026-03-17

### Added

- Added a tiled graph engine with tile IDs, cross-tile routing, border-node indexing and LRU caching.
- Added sport-specific routing profiles and cost factors.
- Added a routing profile editor and engine toggle.
- Added local BRouter data build support from map files.
- Added elevation reading from HGT files.
- Added GPS correction, speed smoothing, distance gap filling and elapsed-time correction.
- Added a unified tag system with an activity-tag junction table and persistent removals.
- Added rule actions and conditions for fields, sun elevation and start/end distance.
- Added an electric-bike system tag and filters.
- Added tag cover thumbnails.

### Changed

- Routing memory usage was reduced through graph streaming, compressed structures and memory-mapped sections.
- Automation rules now apply to non-GPS activities as well.

### Fixed

- Chart zoom synchronization no longer cascades or hangs.
- Map dragging and snap zones were improved.
- Segment navigation queries and weather backfill were corrected.
- Missing tag automation columns and skipped tag schema updates were repaired.

---

## 2026-03-16

### Added

- Added weather enrichment with configurable preferences, weather tags and backfill jobs.
- Added Go-based stream enrichment for stops, ascents and intervals.
- Added ML service capability detection.
- Added a Go analytics package for readiness, anomaly, projection and correlation calculations.
- Added graph edge geometry and elevation profiles.
- Added a unified import log for all importers.
- Added activity trim and split editor improvements.
- Added unified tag rendering.
- Added additional map-data columns in graph tiles.

### Fixed

- Weather model selection and labels were corrected.
- Stream enrichment and ML capability race conditions were fixed.
- Routing graph build memory usage was reduced.
- Import lifecycle and consistency issues were fixed.
- Activity distance handling across lap resets and multisession files was improved.
- Map tile sources were corrected.

---

## 2026-03-15

### Added

- Added per-region map-data import with staging and transactional merge.
- Added scheduled map-data updates with patching support.
- Added preprocessing with tag filtering and merge steps.
- Added more noise source categories.
- Added browser notifications for completed jobs.
- Added region-filtered enrichment.
- Added tag cover image groundwork.
- Added trigram search for addresses and places.

### Fixed

- Map-data geometries now use the correct coordinate system.
- Enrichment polygon joins handle invalid geometries more safely.
- Forest coverage is clamped to avoid values above 100%.
- Region category state is replaced instead of accumulated on reimport.
- Startup resets import-specific database tuning.
- FIT and TCX speed and distance can be derived from GPS when missing.
- Tag activity counts are repaired after activity deletion.
- Notification API calls use the authenticated client.

---

## 2026-03-14

### Added

- Added the core A* routing engine with multi-pass search, elevation-aware costs and GeoJSON output.
- Added mixed-geometry POI support for map data.
- Added landuse, railway and piste area import support.
- Added sport conflict detection and resolution for imports.
- Added retry endpoints for map-data enrichment and topology steps.
- Added improved job indicators and compressed job data.
- Added raster-based enrichment for forest, protected areas and land use.

### Fixed

- Route membership now uses relation members where available.
- Geometry snapping tolerance was corrected.
- Region status is marked before clearing data to improve crash recovery.
- Routing handlers were hardened and input ranges validated.
- A water-sport mapping issue in imports was corrected.

---

## 2026-03-13

### Added

- Added the graph engine data structures, topology builder, tag encoding, elevation sampling, serialization and rebuild status APIs.
- Added spatial enrichment for forest, noise, traffic, protected areas, land use and route membership.
- Added elevation-data schema and fallback handling.
- Added a map-data region manager with status, disk usage, cancel and delete endpoints.
- Added offline map capabilities and style support.
- Added assistant routing tools for POI search, trail search, route planning, isochrones and profile selection.
- Added address and place search.
- Added routing profiles and BRouter build scripts.
- Added road tile sources and wind turbine import as a noise source.

### Fixed

- Spatial enrichment now uses the correct coordinate system.
- Routing profiles were aligned between frontend and backend.
- Elevation tile metadata is populated after import.
- Long map-data jobs remain visible.
- Large imports avoid memory-heavy cache settings.
- Schema-drift-sensitive topology code now builds column lists dynamically.

---

## 2026-03-12

### Added

- Added a central import service for activities, photos, orphan photos and queued photo resolution.
- Added perceptual photo hashing for image and video deduplication.
- Added an import queue for orphan photos.
- Added a unified map-data schema with way, POI, route, piste, glacier and wetland support.
- Added a map-data import assistant with category selection.
- Added platform deeplinks on activity details.
- Added persisted credentials for the Komoot importer when enabled.
- Added visibility mapping from imported activity data.

### Fixed

- Multisport FIT import now handles multi-session files and same-sport sessions correctly.
- Source sport priority and device-specific sport fallback were improved.
- Credential encryption and lifecycle handling were hardened.
- Distance offset handling across lap resets was corrected.
- Route geometry inserts are forced to two-dimensional geometry.
- Map tile configuration was updated for the new map-data schema.

---

## 2026-03-11

### Added

- Added the Komoot importer for activities, photos and planned routes.
- Added Garmin courses import into planned routes.
- Added Strava routes import into planned routes.
- Added route duplicate detection using source identifiers, bounding boxes and shape similarity.
- Added larger photo previews and configurable video preview frame handling.

### Fixed

- Tag activity counts now update correctly.
- Tag text color is calculated for readability.
- Frost automation field names were corrected.
- Electric-bike detection uses a flag instead of a separate sport type.
- Komoot sport mapping and generic FIT fallback behavior were corrected.
- Route duplicate detection and route-import settings were refined.

---

## 2026-03-10

### Added

- Added routing and geodata architecture including BRouter proxying, isochrones, elevation data, route CRUD, automatic route suggestions, POI search and route planner UI.
- Added route photo and tag management, GPX export, route duplication and route creation from activities.
- Added activity elevation and distance correction actions.
- Added runtime capabilities for routing, elevation data, isochrones, offline maps and surface enrichment.
- Added local map and geodata status settings.
- Added fusion merge that creates a new merged activity and keeps source activities restorable.
- Added fusion revert and delete-source choices for fusion and multisport parents.
- Added cached Terra Incognita coverage.
- Added photo previews, authenticated image loading and manual photo geolocation.
- Added duplicate detection for photo and gear document uploads.
- Added Strava media duplicate detection and improved media assignment.
- Added a dedicated fusion workshop page and post-import fusion notices.
- Added upload progress reporting for archive uploads.
- Added multisport detail views, combined charts, map hover synchronization, background recompute and ZIP export.
- Added configurable enrichment module toggles.
- Added semantic photo search, text embedding endpoints and a photo feed.
- Added video, HEIC and JXL support for media uploads.
- Added local LLM chat improvements, tool access and token/context usage display.

### Changed

- Duplicate uploads and already-running recomputes return structured success-like statuses instead of hard HTTP failures.
- GPS similarity now uses overlap percentage rather than only shape distance.
- Fusion candidates are based on real time overlap where possible.
- Import wizard review moved to the fusion workshop page.
- Activity import detection counts only activity FIT files for the activity importer.
- Terra Incognita now shows all activities without the old filter panel.
- Activity lists hide child activities by default.

### Fixed

- BRouter proxy inputs are allowlisted and bounded.
- Ownership checks were added for POI search and route photos.
- Elevation and topology handlers no longer leak internal errors to clients.
- Route suggestion, GPX download, route save dialog, map style switching and Terra Incognita rendering were fixed.
- Garmin and Strava jobs are visible in the active jobs indicator.
- Multisport export, edit buttons and recompute response handling were fixed.
- Large map-data imports no longer overflow progress counters.
- Heatmap tile optimization that caused gaps was reverted.
- Indoor chart X-axis defaults use GPS availability more reliably.

### Security

- BRouter proxy requests use a profile allowlist and bounded alternatives.
- Route photo and media serving enforce ownership and user prefixes.
- URL imports have hardened SSRF protection.

---

## 2026-03-09

### Added

- Added advanced analytics with HRV analysis, feature engineering, change-point detection, readiness, anomaly detection, clustering, correlations and forecasts.
- Added mobile API readiness with PKCE token exchange, device registration, health ingest and notifications.
- Added time-series forecasting models with lazy loading and prediction endpoints.
- Added route similarity analytics.
- Added streaming chat sessions with rate limiting and message history.
- Added a daily feature store.
- Added a configurable chat assistant pipeline with profiles, routing, skills, tool definitions, SQL validation and Markdown rendering.
- Added chat profile settings in the frontend.

### Changed

- The ML container was updated to a newer Python runtime.
- Health snapshot storage was renamed and extended with additional fields.
- Garmin health snapshot imports now read more wellness fields.

### Fixed

- ML settings routes were moved to the user settings path used by the frontend.
- Job streaming now sends authorization headers correctly.
- SQL column names in assistant tools match the current health snapshot schema.
- Forecast inference no longer blocks the ML service event loop.
- Health snapshot processing handles nulls more safely.

### Breaking Changes

- The health snapshot table name changed as part of the schema update.

---

## 2026-03-08

### Added

- Added FIT-level fusion and a fusion page with multi-source track overlays and stream-source selection.
- Added multisport parsing with parent/child activity models.
- Added manual grouping and ungrouping of activities.
- Added combined maps for multisport parents.
- Added a map-based activity editor with visual trim and GPS outlier markers.
- Added local vector-map tiles as selectable map styles.
- Added Terra Incognita map view.
- Added URL import handling with server-side safety checks.
- Added multisport export, import progress updates, power-duration charts, summit filters and expanded health charts.
- Added document processing notifications and document review auto-refresh.

### Changed

- Map styles are now shared across map components.
- Segment editor UI and map style handling were refreshed.
- Race and interval indicators moved toward tag-based representation.

### Fixed

- Ownership checks were added to multiple activity detail, stream, sensor, gear and segment handlers.
- Media serving and heatmap tiles now enforce user scope.
- Side-effect and stale-closure issues in enrichment progress and map overlays were fixed.
- Fusion stitching and competition tags now use more efficient and consistent data writes.
- Multisport UI, API calls and map layer race conditions were corrected.

### Security

- Path traversal in the file shadow store and SSRF risks in URL imports were hardened.
- Heatmap tile functions now filter by user.

### Removed

- Removed a legacy race column in favor of tags.
- Removed an unused enrichment module.

---

## 2026-03-07

### Added

- Added a unified analytics filter panel with sport hierarchy, search and tri-state tag/gear filters.

### Changed

- Commute detection moved from a boolean field to the commute tag system.
- Several runtime dependencies were updated.

### Fixed

- Training monotony and monotony formula calculations were corrected.
- Recompute-all progress now reaches 100% even when individual activities fail.
- Segment distance uses geodetic length.
- Import redirects, request body serialization and gear update methods were corrected.
- Map filters now reload tiles correctly after tag or gear changes.
- Heatmap, Terra Incognita and map style overlay updates were fixed.
- Import progress and module error reporting were corrected.
- Segment recompute now passes a valid user context.

---

## 2026-03-06

### Added

- Added cursor-based segment pagination and infinite scroll.
- Added segment filters for ridden-only, sport groups and terrain types.
- Added a segment detail page with minimap, performance chart and activity list.
- Added segment overlays on the main map with favorites and sport filters.
- Added OSM import assistant progress steps.
- Added global background job tracking and active job indicators.
- Added persistent status for segment and activity recompute jobs.
- Added Strava photo import.
- Added calendar year-picker and health overlays.
- Added a reusable modal system and toast notifications.

### Fixed

- Export actions now use authenticated downloads and correct filenames.
- Heatmap line tiles use precomputed projected geometry and new indexes for performance.

---

## 2026-03-05

### Added

- Added persistent map viewport and default map style settings.
- Added redesigned location pin popups with images, photo gallery, color picker and linked activities.
- Added magnetic drag-and-drop assignment for activities and location pins.
- Added custom map marker editing and images.
- Added system weather tags and weather-tag normalization.
- Added full reprocessing controls for enrichment modules.
- Added map-based segment editor, lap detection, trail difficulty, favorites and segment detail views.
- Added automation rules with condition/action builder, geofences, segment conditions and pace calculation.
- Added gastronomy stop detection, visited POI endpoints and map overlays.
- Added an import hub in the header.
- Added lift candidates, lift segment import and lift statistics controls.
- Added health monitoring imports from Garmin archive data.
- Added browser ZIP uploads for Garmin and Strava archives.

### Fixed

- Location pin state refreshes correctly after tag changes.
- Drag-and-drop map timing issues were fixed.
- Lift segment import now uses the correct map-data identifier.
- Gastronomy and stop detection database constraints were corrected.
- Automation rule ownership checks were added.
- Lift candidate confirmation and rejection update aggregate statistics.
- Body battery, monitoring FIT classification and import buttons were fixed.

---

## 2026-03-04

### Added

- Added an analytics page with training, health, statistics and insights tabs.
- Added a global analytics filter panel with presets, custom ranges and saved filter presets.
- Added new analytics API endpoints.
- Added running dynamics, respiration and RR-interval parsing.
- Added ML-based readiness, anomaly detection, clustering and insights.
- Added dashboard redesign with readiness and compact activity cards.
- Added polygon privacy zones and color settings for privacy zones.
- Added split difficulty editing for climbing activities.
- Added hotfolder imports through the central upload path.
- Added GPS-similarity duplicate detection and a fusion queue.
- Added duplicate review in the import wizard.
- Added Garmin and Strava archive importers for profile, gear, activities and health data.
- Added import job status with logs, progress and module tracking.
- Added document processing, document review and LLM settings UI.
- Added multi-source import conflict strategies and activity enrichment/replacement flows.

### Fixed

- Garmin archive import parsing issues for gear, thresholds, weight and purchase dates were fixed.
- Imported activity title fallback now uses automatic titles when source titles are missing.
- HRV charts and analytics now read the correct source data.
- Yearly overview counts, accepted empty responses and recommendation language were corrected.
- Indoor activities without GPS hide GPS-only sections.
- Privacy-zone wording was standardized.

---

## 2026-03-03

### Added

- Added dashboard widgets, calendar views, climb tables, grouped geo tags and gear document upload/review.
- Added activity-list page memory, right-click actions and reprocessing dialogs.
- Added a reusable color picker with hex input.
- Added tag merge and location sync actions.
- Added lap, split, swim length and climb event tables and endpoints.
- Added custom maps, map markers and activity photos.
- Added heatmap glow, configurable track colors, custom map panels and photo overlays.
- Added multi-select activity actions, bulk export, batch recompute and batch delete.
- Added automatic segment backfill and creating segments from activities.
- Added global sport color settings and a hide-unused-sports option.
- Added support for storing ANT+ device numbers.
- Added rule conversion and manual geofence radius input.
- Added year calendar redesign and navigation.

### Fixed

- Sport filters no longer send subtype filters when only a parent sport is selected.
- Export URL construction no longer duplicates the API prefix.
- Lap tables handle missing indexes safely.
- JSON field names for lap and split responses were corrected.
- Map drag-and-drop and location-pin reset logic were fixed.
- Location tag activities can now be listed.
- Component image limits and supported formats were expanded.
- Map style race conditions and dark-mode filter panel rendering were fixed.
- Sensor labels and hide-unused-sports settings are applied correctly.
- Automation rule deletion, geofence editing and gear preset handling were fixed.
- Bulk ZIP downloads and multi-tab opening behavior were improved.
- Hotfolder settings are persisted.

---

## 2026-03-02

### Added

- Added full single-activity recompute from original files.
- Added filtered recompute-all jobs with selectable scopes.
- Added activity list sorting, date jumping and sortable columns.
- Added full-screen activity maps with synchronized charts.
- Added synchronized chart brush zoom, time/distance axis toggles, hover map markers, temperature charts and min/max annotations.
- Added moving-time calculation from streams with sport-specific thresholds.
- Added extended performance-management chart ranges and clickable activity rows.

### Fixed

- Brush zoom no longer causes browser hangs or missing data points.
- Chart-to-map hover markers use the correct coordinate index.
- Full-screen map initialization waits until the container has dimensions.
- Date-jump cursor construction no longer uses an invalid identifier.
- Activity checkbox events no longer conflict with row navigation.
- Performance-management chart date scanning and full-history recalculation were fixed.

---

## 2026-03-01

First Prototype

### Added

- Added FIT, GPX and TCX activity parsing.
- Added automatic sport-type classification and manual-classification fallback.
- Added source and recording-device detection.
- Added stream data, GPS tracks and sensor recognition.
- Added the initial database, storage and event infrastructure.
- Added file deduplication by SHA-256 hash.
- Added upload, list and activity detail APIs.
- Added initial authentication, refresh, logout and development-auth flows.
- Added performance-management calculations and asynchronous recomputation.
- Added the initial activity detail page, charts, map integration, dashboard and activity list.

### Changed

- API routing was split into public authentication routes and protected application routes.

### Fixed

- Missing initial database columns and performance-management tables were added.
- Initial authentication startup issues were fixed.
