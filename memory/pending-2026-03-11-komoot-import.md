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
