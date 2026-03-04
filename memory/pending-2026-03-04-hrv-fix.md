## docs/04_ML_AI_SERVICE.md — HRV-Datenquelle
**Suche nach:** HRV
**Delta:** Kein Änderungsbedarf — betrifft nur Backend-DB-Query, nicht ML-Service

## docs/08_API_REFERENCE.md — GET /analytics/hrv Response
**Suche nach:** /analytics/hrv
**Delta:** Response-Schema geändert: `morning_avg` und `night_avg` Felder entfernt. HRVPoint: `sdrr` und `time_of_day` Felder entfernt.

## docs/07_DATABASE_SCHEMA.md — HRV-Datenquelle
**Suche nach:** health_hrv_snapshots
**Delta:** `GetHRVAnalysis` liest jetzt aus `health_readiness_scores.hrv_weekly_avg_ms` statt `health_hrv_snapshots`. Gilt auch für `GetHRVCorrelation` und `healthMetricFromHRV`. `health_hrv_snapshots` bleibt in der DB erhalten (ältere Wellness-Snapshots 2023–2024), wird aber nicht mehr für Analytics-Charts genutzt.

## CHANGELOG.md
**Delta:**
- fix(analytics): HRV-Analyse liest jetzt aus health_readiness_scores.hrv_weekly_avg_ms statt health_hrv_snapshots — behebt leeren HRV-Chart im 90d-Zeitfenster
- Betroffen: database/analytics_health.go (GetHRVAnalysis, GetHRVCorrelation, healthMetricFromHRV), handler/analytics/health.go (morning_avg/night_avg entfernt), frontend api/analytics.ts (HRVPoint/HRVAnalysis Interfaces), HRVAnalysis.tsx (Morgen/Nacht-Avg-Badges entfernt)
