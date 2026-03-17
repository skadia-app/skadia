## frontend/src/api/enrichment.ts — backfillWeather return type
**Suche nach:** backfillWeather
**Delta:** Return-Typ von `{ processed: number; errors: number; error_details?: string }` auf `{ job_id: string; status: string }` korrigiert — Backend gibt HTTP 202 + async job zurück, nicht mehr synchrones Ergebnis.

## frontend/src/pages/EnrichmentSettingsPage.tsx — Wetter-Backfill Success-Meldung
**Suche nach:** weatherMut.isSuccess
**Delta:** Success-Meldung von "Wetter-Backfill abgeschlossen: {processed} verarbeitet" auf "Wetter-Backfill gestartet — läuft im Hintergrund" geändert, da der Job async läuft und kein processed-Zähler mehr im Response ist.
