# Pending: Activity Detail Overhaul

## Deltas

### docs/CHANGELOG.md
- Neu anlegen: Erster Eintrag für Activity Detail Overhaul (2026-03-28)
- 22 Tasks: sportspezifische Labels, neue Datenvisualisierungen, Komponenten-Extraktion, Chart-Tabs, sportspezifische Stat-Komponenten

### docs/03_FRONTEND.md

#### Abschnitt "Aktivitäts-Detail-Charts"
- Datei `src/components/ActivityCharts.tsx` existiert weiterhin, aber Charts sind jetzt in ein **Tab-System** (`ChartTabs.tsx`) eingebettet
- Neue Charts: Steigungsfärbung im Höhenprofil (8-Farben ECharts visualMap), Running-Dynamics-Streams (GCT, VO, Schrittlänge), Pedal-Analytics (Drehmoment-Wirkungsgrad + Gleichmäßigkeit L/R), E-Bike-Charts (Motorleistung, Akku, Reichweite), Tiefenprofil (Tauchen)
- Speed ↔ Pace Toggle mit localStorage-Persistenz pro segmentGroup

#### Neuer Abschnitt: "ActivityDetailPage — Komponenten-Architektur"
- `ActivityDetailPage.tsx` wurde refactored; extrahierte Komponenten unter `components/activity/`:
  - `StatsGrid.tsx` — sportspezifisches Stats-Grid mit 12 neuen Metrik-Feldern (Kalorien, Arbeit, Zyklen, IF, L/R-Balance etc.)
  - `DataCorrections.tsx` — Höhe/Distanz-Korrekturen
  - `VisibilitySettings.tsx` — DataVisibility-Toggles
  - `SunInfo.tsx` — Sonnenauf/-untergang
  - `SensorList.tsx` — ANT+/BLE Sensoren
  - `LocationLine.tsx` — Start→Ziel Ortsanzeige
  - `ChartTabs.tsx` — Tab-Infrastruktur (Radix UI Tabs, URL-State `?tab=leistung`, CSS-Visibility für Performance)
  - `PiPMap.tsx` — SVG Picture-in-Picture Minikarte mit IntersectionObserver + Downsampling
  - `SpeedPaceToggle.tsx` — Speed ↔ Pace Umschalter

#### Neuer Abschnitt: "Sport-spezifische Komponenten"
- `StrengthSetsTable.tsx` — Krafttraining-Sets Tabelle mit 38 deutschen Übungsbezeichnungen
- `WorkoutOverview.tsx` — Workout-Step Visualisierung (horizontal)
- `RunningDynamicsStats.tsx` — Running Dynamics Stats-Zeile (GCT, VO, Schrittlänge)
- `DiveStats.tsx` — Tauch-Basis-Metriken
- `BallSportStats.tsx` — Ballsport-Metriken (Score, Ballgeschwindigkeit)
- `JumpSection.tsx` — Sprung-Metriken + aufklappbare Tabelle
- `RadarSummary.tsx` — Radar-Threats Info-Zeile (Varia Fahrzeuganzahl)

#### Abschnitt "Sport Taxonomy"
- `segmentGroup` als neues Feld in `SportDef` hinzugefügt (bike/foot/swim/ski/paddle/generic)
- Alle 83 Sport-Einträge wurden mit `segmentGroup` annotiert

#### Abschnitt "Tech-Stack"
- Neue npm-Abhängigkeit: `@radix-ui/react-tabs`

### docs/09_FEATURE_CATALOG.md (Abschnitt 1. Aktivitäts-Management)

#### Zeile "Aktivitäts-Detail"
- Status auf ✅ setzen
- Beschreibung erweitern: "Karte, Höhenprofil mit Steigungsfärbung, sportspezifische Charts (Tab-System), sportspezifische Stat-Komponenten, Running Dynamics, E-Bike, Schwimmen, Krafttraining, Workout, Tauchen, Ballsport, Sprünge, Radar"

### docs/10_DEVELOPMENT_PLAN.md
- Activity Detail Overhaul als abgeschlossen markieren (falls dort aufgeführt)
