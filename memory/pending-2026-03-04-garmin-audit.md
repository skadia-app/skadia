## Garmin-Gesundheitsdaten Vollaudit — 2026-03-04

### Audit-Ergebnisse (vollständig)

**Korrekt implementiert:**
- HRV Analytics `/analytics/hrv` liest korrekt aus `health_readiness_scores.hrv_weekly_avg_ms` (1043 Einträge)
- Schlaf-Import komplett (1042 Einträge, alle Felder incl. sleep scores)
- Gewicht korrekt in Gramm gespeichert (Scale-Faktor OK, /1000 beim Lesen)
- Blutdruck-Import korrekt (3 Einträge aus BloodPressureFile)
- Training Readiness korrekt importiert aus `DI-Connect-Metrics/TrainingReadinessDTO_*.json`
- Training Load (ATL/CTL) korrekt importiert aus `DI-Connect-Metrics/MetricsAcuteTrainingLoad_*.json`
- Stress täglicher Durchschnitt korrekt aus UDS allDayStress.aggregatorList[TOTAL]
- Analytics `/analytics/hrv` und `/analytics/hrv/recovery` funktionieren korrekt

---

### Behobene Bugs (in diesem Commit)

**Bug 1: Body Battery Parser falsch**
- **Datei:** `services/api/internal/garmin/importer_steps.go`
- **Suche nach:** `parseBodyBattery`
- **Problem:** Parser erwartete `bodyBatteryValuesArray` (veraltet), echte Garmin-Exports haben `bodyBatteryStatList` mit `HIGHEST`/`LOWEST` StatTypes
- **Fix:** `parseBodyBattery` auf `bodyBatteryStatList`-Format umgestellt
- **Effekt:** `health_daily_summary.body_battery_min/max` war immer NULL, jetzt korrekt befüllt nach Re-Import

**Bug 2: `/health/hrv` liest aus falscher Tabelle**
- **Datei:** `services/api/internal/database/health.go`
- **Suche nach:** `GetHealthHRV`
- **Problem:** `health_hrv_snapshots` hat nur 27 Einträge (bis 2024-05), war die einzige Quelle
- **Fix:** UNION-Query: zuerst `health_hrv_snapshots` (echte Messungen), dann `health_readiness_scores.hrv_weekly_avg_ms` für Tage ohne echten Snapshot
- **Effekt:** HealthDashboardPage HRV-Chart zeigt jetzt >1000 Datenpunkte statt 27

**Bug 3: Stress und SpO2 aus wellnessActivities wurden ignoriert**
- **Datei:** `services/api/internal/garmin/importer_steps.go`
- **Suche nach:** `Abschnitt 3: HRV-Snapshots`
- **Problem:** `STRESS` und `SPO2` Felder aus `HEALTH_MONITORING` Wellness-Records wurden nicht in `health_stress_snapshots`/`health_spo2_snapshots` gespeichert
- **Fix:** Importer speichert jetzt auch Stress und SpO2 pro Snapshot; HRV-Snapshot wird nur gespeichert wenn RMSSD vorhanden (kein Skip-on-nil-Panic mehr)

**Feature: FitnessAge-Import (neu)**
- **Datei:** `services/api/internal/garmin/importer_steps.go`
- **Suche nach:** `Abschnitt 8: Fitness Age`
- **Problem:** `DI-Connect-Wellness/*_fitnessAgeData.json` hatte 774 Einträge (bioAge), wurde nicht importiert
- **Fix:** Neuer Import-Abschnitt 8 liest `currentBioAge` aus `fitnessAgeData.json` → `health_fitness_age`
- Neue JSON-Struct: `garminFitnessAge`

**Feature: Hydration-Import (neu)**
- **Datei:** `services/api/internal/garmin/importer_steps.go`
- **Suche nach:** `Abschnitt 9: Hydration`
- **Problem:** `DI-Connect-Aggregator/HydrationLogFile_*.json` wurde nicht importiert, `health_daily_summary.hydration_ml` war immer NULL
- **Fix:** Neuer Import-Abschnitt 9 aggregiert Tages-Hydration (nur `valueInML > 0`, keine Schweiß-Verluste) → `health_daily_summary.hydration_ml`
- Neue JSON-Struct: `garminHydrationLog`

**Feature: Mehr Metriken in GetHealthMetrics**
- **Datei:** `services/api/internal/database/analytics_health.go`
- **Suche nach:** `GetHealthMetrics`
- **Problem:** `/analytics/health-metrics` lieferte nur RHR, HRV, Schlaf, Gewicht; Frontend-`HealthMetricsGrid` hatte Farben für `steps`, `stress`, `body_battery` - aber diese wurden nie geliefert
- **Fix:** Drei neue Metric-Funktionen: `healthMetricFromSteps`, `healthMetricFromStress`, `healthMetricFromBodyBattery`

---

### Noch offene Punkte (TODO, keine einfachen Fixes)

**MetricsMaxMetData (VO2max):**
- 7+ Dateien in `DI-Connect-Metrics/MetricsMaxMetData_*.json` vorhanden
- Felder: `calendarDate`, `vo2MaxValue`, `maxMet`, `sport`
- Keine DB-Tabelle vorhanden → Migration 000064 notwendig
- Kein Analytics-Endpoint dafür

**RunRacePredictions:**
- 11 Dateien in `DI-Connect-Metrics/RunRacePredictions_*.json` vorhanden
- Felder: `calendarDate`, `raceTime5K/10K/Half/Marathon` (Sekunden)
- Keine DB-Tabelle vorhanden → Migration 000064 notwendig
- Kein Analytics-Endpoint, kein Frontend-Anzeige

**MetricsHeatAltitudeAcclimation:**
- Dateien in `DI-Connect-Metrics/MetricsHeatAltitudeAcclimation_*.json`
- Nicht importiert, keine DB-Tabelle - P4 Feature

---

## docs/02_CORE_BACKEND.md — Garmin Health Importer
**Suche nach:** `StepsImporter`
**Delta:**
- `parseBodyBattery` korrigiert: `bodyBatteryStatList` statt `bodyBatteryValuesArray`
- Wellness Activities: STRESS→health_stress_snapshots, SPO2→health_spo2_snapshots jetzt auch gespeichert
- Neuer Abschnitt 8: FitnessAge aus `*_fitnessAgeData.json` → `health_fitness_age`
- Neuer Abschnitt 9: Hydration aus `HydrationLogFile_*.json` → `health_daily_summary.hydration_ml`

## docs/07_DATABASE_SCHEMA.md — Health Tables
**Suche nach:** `health_daily_summary`
**Delta:**
- `body_battery_min/max` wird jetzt korrekt befüllt (Body Battery Fix)
- `hydration_ml` wird jetzt aus HydrationLogFile befüllt

## docs/08_API_REFERENCE.md — Analytics Endpoints
**Suche nach:** `GET /analytics/health-metrics`
**Delta:**
- `/analytics/health-metrics` liefert jetzt auch: `steps` (steps), `stress` (pts), `body_battery` (pts)
- `/health/hrv` liest jetzt aus UNION von `health_hrv_snapshots` + `health_readiness_scores`

## CHANGELOG.md
**Delta:**
- fix(garmin): korrigiere Body-Battery-Parser (bodyBatteryStatList statt bodyBatteryValuesArray)
- fix(health): /health/hrv UNION-Query auf health_readiness_scores für vollständige HRV-Daten
- feat(garmin): importiere Stress und SpO2 aus Wellness-Monitoring-Snapshots
- feat(garmin): neuer Import für FitnessAge (fitnessAgeData.json → health_fitness_age)
- feat(garmin): neuer Import für Hydration (HydrationLogFile → health_daily_summary.hydration_ml)
- feat(analytics): GetHealthMetrics ergänzt um steps, stress, body_battery Metriken
