## frontend/src/api/komoot.ts — Komoot Frontend API Client (NEU)

**Suche nach:** komoot import
**Delta:**
- Neu: `frontend/src/api/komoot.ts` mit `KomootScanResult`, `KomootLoginResponse`, `KomootJob`, `KomootStartRequest` Typen
- Funktionen: `komootLogin()`, `komootScan()`, `komootStartImport()`, `getKomootImportJob()`

## frontend/src/pages/ImportPage.tsx — Komoot Tab

**Suche nach:** Datenimport
**Delta:**
- Neuer Tab "Komoot" in ImportPage (nach Strava, vor Queue)
- `KomootImportPanel` Komponente: 3-Phasen-Wizard (Login → Konfiguration → Import)
- Login mit E-Mail/Passwort oder gespeicherten Zugangsdaten
- Konfiguration: Aktivitäten/Routen/Fotos toggle + Konflikt-Strategie
- Job-Polling mit Fortschrittsbalken, Log, localStorage-Reconnect

## frontend/src/pages/SettingsPage.tsx — Route-Duplikat-Settings

**Suche nach:** Route-Duplikat
**Delta:**
- `ImportSettingsSection` erweitert: 2 neue Slider für Route-Duplikaterkennung
- Hausdorff-Schwellwert (10–200 m, Default 50 m) via `/settings/enrichment` API
- Längendifferenz-Schwellwert (0,5–10 km, Default 2 km) via `/settings/enrichment` API

## CHANGELOG.md

**Delta:**
- Komoot Import: Login, Scan, geplante Routen, Fotos, Background-Job
- Garmin Courses Importer (geplante Routen aus JSON)
- Strava Routes Importer (geplante Routen aus CSV + GPX)
- Route-Duplikat-Erkennung (3-Tier: komoot_tour_id, hash, Hausdorff+Länge)
- Migration 000143: planned_routes Tabelle, source_platform/source_id/source_url

## services/api/internal/komoot/importer_photos.go — Komoot Photos Importer

**Suche nach:** komoot photos
**Delta:**
- Neu: `PhotosImporter` struct mit `TourActivityMap map[int64]string`
- Import-Flow: GetTourDetail → Timeline-Filter (type=="photo") → DownloadPhoto → SHA256-Dedup → EXIF-Parse → location_source Logic → imageutil.Resize (200px thumb + preview) → S3 → InsertActivityPhoto
- location_source: "exif" wenn EXIF-GPS, "manual" wenn Timeline-Location, "activity_track" als Fallback
- Hilfsfunktionen: `komootPhotoContentType`, `komootImageFormatFromContentType`, `extractKomootEXIF`
- Keine Videos (Komoot CDN liefert nur Bilder)

## services/api/internal/handler/komoot_import.go — Credential Lifecycle

**Suche nach:** komoot credential
**Delta:**
- `save_credentials` Flag auf Login: Credentials immer in DB gespeichert (Import-Goroutine braucht sie), `sync_enabled` = `save_credentials`
- `delete_credentials_after` Flag auf Import-Start: löscht Credentials nach erfolgreichem Import
- `GET /import/komoot/credentials` — neuer Endpoint: gibt `has_credentials` + `email` zurück (leichtgewichtig, kein Komoot-Login)
- `DELETE /import/komoot/credentials` — löscht gespeicherte Credentials
- Panic Recovery: `r` → `rec` (kein Shadowing von `*http.Request`)
- Encryption Key Validation in Login/Scan/Start

## frontend/src/pages/ImportPage.tsx — Credential Persistence UI

**Suche nach:** Zugangsdaten
**Delta:**
- Checkbox "Zugangsdaten dauerhaft speichern" unter Passwort-Feld
- Dynamischer Hilfetext (verschlüsselt gespeichert vs. nach Import gelöscht)
- Gespeicherte E-Mail + roter "Löschen"-Button wenn Credentials existieren
- "Gespeicherte Zugangsdaten nutzen" nur sichtbar wenn Credentials in DB

## frontend/src/components/ImportHub.tsx — Komoot Entry

**Suche nach:** ImportHub
**Delta:**
- KomootBadge (grünes "K", Farbe #6AA127) + Button in BULK-IMPORTER Sektion

## Projektweiter Fix: accent-orange-500 → accent-accent-DEFAULT

**Suche nach:** accent-orange
**Delta:**
- Alle `accent-orange-500` (hardcoded) durch `accent-accent-DEFAULT` (CSS Custom Properties aus Akzentfarben-System) ersetzt
- Betroffene Dateien: ImportPage, SettingsPage, ActivitiesPage, RuleEditor, MapFilters, RecomputeModal, ZoneEditor, PrivacyZoneEditor
- 0 Vorkommen verbleibend in Code-Dateien (nur noch in docs/internal)

## services/api/internal/database/routes.go — Kosmetik

**Delta:**
- `fmt.Sscanf` → `strconv.ParseInt` in `GetKnownKomootRouteIDs` (idiomatisches Go)
