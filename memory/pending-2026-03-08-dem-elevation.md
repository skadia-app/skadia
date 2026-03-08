## services/api/internal/migrate/migrations/ — Migration 000098
**Suche nach:** 000098
**Delta:** Migration 000098_elevation_source: `activities.elevation_source TEXT DEFAULT 'device'`; Werte: device/dem/manual

## services/api/internal/enrichment/elevation_dem.go — neu
**Suche nach:** FetchDEMElevations
**Delta:** Neues File. FetchDEMElevations(ctx, lats, lngs) → Open-Meteo /v1/elevation, Batches à 100 Punkte, 10s Timeout. applyDEMCorrectionToStreams: >50m Abweichung → barometrischen Wert beibehalten.

## services/api/internal/enrichment/stage1.go — DEM-Aufruf
**Suche nach:** DEM-Höhenkorrektur
**Delta:** Neuer Block vor Reverse-Geocoding: wenn HasGPS && len(Streams)>0 → FetchDEMElevations → applyDEMCorrectionToStreams → fields["elevation_source"]="dem"; bei Fehler "device"

## services/api/internal/model/activity.go — ElevationSource Feld
**Suche nach:** ElevationSource
**Delta:** Neues Feld `ElevationSource string json:"elevation_source,omitempty"` am Ende des Activity-Structs

## services/api/internal/database/database.go — GetActivity Query
**Suche nach:** COALESCE(a.elevation_source
**Delta:** GetActivity SELECT + Scan um `COALESCE(a.elevation_source, 'device')` / `&a.ElevationSource` erweitert

## frontend/src/api/types.ts — Activity Interface
**Suche nach:** elevation_source
**Delta:** `elevation_source?: string | null` zum Activity Interface hinzugefügt

## frontend/src/pages/ActivityDetailPage.tsx — DEM Badge
**Suche nach:** DEM badge
**Delta:** StatsGrid: badge-Feld in stats-Array; bei elevation_source==='dem' zeigt "Anstieg"-Kachel kleinen grünen "DEM"-Badge an (title="Höhe via DEM (Open-Meteo) korrigiert"). ReactNode aus react importiert.

## CHANGELOG.md
**Suche nach:** Unreleased
**Delta:** feat(enrichment): DEM-Höhenkorrektur via Open-Meteo — elevation_source Spalte, FetchDEMElevations Batch-Client, Stage-1 Integration, DEM-Badge in ActivityDetailPage StatsGrid
