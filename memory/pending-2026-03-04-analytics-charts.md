## frontend/src/components/analytics/training/TrainingVolumeChart.tsx — Line/Balken-Toggle + Zoom
**Suche nach:** TrainingVolumeChart
**Delta:**
- `ChartType = 'bar' | 'line'` State hinzugefügt
- `LineChart` und `ToolboxComponent` importiert und registriert
- Series-Erstellung bedingt: Balken gestapelt (Linie ohne Stack, `connectNulls: false`)
- `toolbox.feature.dataZoom` (Brush-Zoom) + `toolbox.feature.restore` hinzugefügt
- `dataZoom: [{ type: 'inside' }]` war schon vorhanden, bleibt erhalten
- Toggle-Button "Balken / Linie" in der Toolbar ergänzt

## frontend/src/components/analytics/training/YearProgressionChart.tsx — Brush-Zoom
**Suche nach:** YearProgressionChart
**Delta:**
- `ToolboxComponent` importiert und registriert
- `toolbox.feature.dataZoom` (Brush-Zoom, yAxisIndex: 'none') in sharedConfig eingefügt
- `dataZoom: [{ type: 'inside' }]` in sharedConfig eingefügt (für beide Charts: monatlich + kumulativ)

## frontend/src/components/analytics/training/ZoneDistributionChart.tsx — Brush-Zoom im Balken-Modus
**Suche nach:** ZoneDistributionChart
**Delta:**
- `DataZoomComponent` und `ToolboxComponent` importiert und registriert
- `toolbox.feature.dataZoom` (Brush-Zoom) + `dataZoom: [{ type: 'inside' }]` in bar-Modus-Option eingefügt
- Pie-Modus bleibt unverändert (kein Zoom sinnvoll)

## frontend/src/components/analytics/training/PersonalRecordsTable.tsx — Backend-Keys in RECORD_TYPE_LABELS
**Suche nach:** RECORD_TYPE_LABELS
**Delta:**
- `max_distance` → "Längste Distanz" ergänzt (Backend liefert diesen Key)
- `max_elevation` → "Meiste Höhenmeter" ergänzt (Backend liefert diesen Key)
- `max_avg_power` → "Max. Durchschnittsleistung" ergänzt (Backend liefert diesen Key)
- `fastest_pace` war bereits vorhanden und korrekt
- Alte/Legacy-Keys bleiben zur Vorwärtskompatibilität erhalten

## Nicht geändert (bereits korrekt)
- `PMCChartExtended.tsx`: Hat bereits vollständigen Zoom (toolbox + dataZoom inside + slider)
- `HRVAnalysis.tsx`: Hat bereits `dataZoom: [{ type: 'inside' }]`; kein toolbox-Zoom ergänzt (nur Scatter-Punkte, keine Zeitachsen-Balken)

## CHANGELOG.md
**Neue Einträge unter [Unreleased]:**
```
### Fixed
- analytics: Persönliche Bestleistungen zeigen jetzt deutsche Labels statt DB-Keys (max_distance, max_elevation, max_avg_power)

### Added
- analytics: Brush-Zoom (Toolbox + Inside-Zoom) zu TrainingVolumeChart, YearProgressionChart und ZoneDistributionChart (Balken-Modus) hinzugefügt
- analytics: Line/Balken-Toggle zu TrainingVolumeChart ergänzt
```
