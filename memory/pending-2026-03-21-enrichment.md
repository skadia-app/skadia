## docs/00_overview.md — Enrichment Quality Redesign
**Suche nach:** Enrichment
**Delta:** Unified Elevation Filter (resample 5m + SG). POI-first Stop Detection (Spatial Confinement, 11 Kategorien). FIT-Splits Interval Tier-1. Sport-adaptive Climbs (S1-S5). Hybrid Settlements (Boundaries + POI). Re-Enrich Endpoint. Recompute async.

## CHANGELOG.md
**Suche nach:** Unreleased
**Delta:**
### Added
- Unified Elevation Filter: Resample 5m + Savitzky-Golay(7, poly=2) + Hysteresis (2m/3m/5m per source)
- POI-first Stop Detection: Spatial Confinement (Li/Zheng), 11 POI-Kategorien, Speed-Fallback
- FIT-Splits Interval Detection (Tier-1): Zero False Positives fuer Garmin-Aktivitaeten
- Sport-adaptive Climb Classification: S1-S5 (Wandern), HC-4 (Rad), leicht/mittel/schwer (Generisch)
- Hybrid Settlement Detection: admin_level=8 Boundaries + POI-Fallback, geometrische Deduplizierung
- Re-Enrich Endpoint: POST /api/v1/activities/re-enrich (Modul-Selektor, 3 Worker)
- EnsureActivityTrack: INSERT ON CONFLICT fuer fehlende Tracks
### Changed
- Single-Activity Recompute laeuft async (context.Background + Background-Job + 202)
- Interval Heuristic: Sport-Whitelist, Confidence Score, CV-Check, Retraction
- FilterElevationGainLoss: Neue Signatur mit distances[], delegiert an SmoothElevation
### Fixed
- 1900% Gradient bei Anstiegen (Resample eliminiert Micro-Segmente)
- 61 Micro-Stopps (POI-first statt Speed-first)
- Falsche "Intervalltraining"-Tags bei Wanderungen (Sport-Gate + Confidence >= 0.7)
- Grossstaedte nicht als Ortschaft erkannt (Boundary-basiert statt nur POI-Radius)
- 722 Aktivitaeten ohne Track (Bulk-Rebuild via SQL)
- Recompute abgebrochen bei Client-Disconnect (context.Background)
