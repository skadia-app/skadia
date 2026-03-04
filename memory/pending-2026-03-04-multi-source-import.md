## docs/02_CORE_BACKEND.md — Multi-Source Import System
**Suche nach:** Garmin Import
**Delta:**
- Neues `importer` Package: DuplicateDetector (CheckTier1: hash/platform_id/temporal), Enricher (EnrichActivity/ReplaceActivity), ImportOptions
- DB Migration 000060: activities.fusion_sources JSONB, activities.is_fusion_source BOOL, users.import_conflict_strategy, users.gps_duplicate_threshold
- Garmin + Strava Importer nutzen zentralen DuplicateDetector mit ConflictStrategy
- Upload Handler: smart duplicate handling statt 409
- Pre-Scan Endpoints: POST /api/v1/import/garmin/scan + /strava/scan
- Fusion DB: SetFusionSources, GetFusionSources, MarkAsFusionSource, FindFusionCandidates
- Fusion Handler: POST /activities/{id}/fusion, GET /activities/{id}/fusion-candidates
- Streams: fusion-aware (applyFusionSources für GPS/HR/Power overlay)
- Gear-Merge: MergeGear TX + FindGearDuplicates (levenshtein ≤ 3)

## docs/03_FRONTEND.md — Multi-Source Import System
**Suche nach:** ImportPage
**Delta:**
- ImportPage: 4-Phasen-Wizard (Scan → Konfiguration → Import)
- Neue Komponente: GearMergePanel (Duplikate zusammenführen)
- SettingsPage: ImportSettingsSection (conflict_strategy + gps_duplicate_threshold)
- Neues API-Client: frontend/src/api/importWizard.ts

## CHANGELOG.md
**Delta:**
### Added
- Multi-source import system mit intelligent duplicate detection (tier-1/tier-2)
- Import wizard mit 4-Phasen-Flow (pre-scan, category config, conflict strategy, import)
- Activity enrichment: fehlende Felder aus zweiter Quelle ergänzen
- Virtual activity fusion mit fusion_sources JSONB
- Gear merge panel mit Levenshtein-basierter Duplikaterkennung
- Import-Einstellungen in SettingsPage (conflict strategy + GPS threshold)
- Pre-scan endpoints für Garmin + Strava Export-Analyse
