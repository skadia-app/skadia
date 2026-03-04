## docs/03_FRONTEND.md — Analytics API client
**Suche nach:** apiClient 204
**Delta:** `apiClient` gibt bei Status 202 (wie 204) `undefined` zurück statt `response.json()` aufzurufen. Fix: Bedingung erweitert zu `response.status === 204 || response.status === 202`.

## docs/02_CORE_BACKEND.md — YearlyOverview JSON-Key
**Suche nach:** YearlyOverview
**Delta:** `database.YearlyOverviewRow.Activities` hat jetzt JSON-Tag `"count"` statt `"activities"`, damit es mit dem TypeScript-Interface `YearlyOverviewRow.count` übereinstimmt. Vorher: NaN-Anzeige in Jahresübersicht.

## docs/02_CORE_BACKEND.md — Recovery-Level Texte
**Suche nach:** recoveryLevel
**Delta:** `recoveryLevel()` in `handler/analytics/health.go` gibt jetzt deutsche Empfehlungstexte zurück statt englischer Texte.

## CHANGELOG.md
**Neu unter Unreleased:**
- fix(analytics): Jahresübersicht zeigte "NaN Aktivitaten" — YearlyOverviewRow JSON-Key von `activities` auf `count` korrigiert
- fix(analytics): "Insights aktualisieren" schlug fehl — apiClient ignoriert jetzt 202 Accepted ohne Body (wie 204 No Content)
- fix(analytics): Recovery-Score zeigte englische Empfehlungstexte — recoveryLevel() auf Deutsch umgestellt
