## frontend/src/components/StaticTrackMap.tsx — Neue Komponente
**Neu erstellt**
**Delta:** Statische Karte mit OSM-Kacheln (keine MapLibre-Abhängigkeit): `<image>`-Elemente in SVG + `<polyline>` Track-Overlay. Web-Mercator-Projektion (lngToWorldX/latToWorldY). Zoom-Auswahl: iteriert von z=17 bis z=1, stoppt wenn Track ≤72% der ViewBox füllt (500×300 SVG-Units). ViewBox zentriert auf Track-Mittelpunkt. 2–6 Kacheln werden automatisch berechnet. OSM-Attribution pflichtgemäß eingebunden.

## frontend/src/components/ActivityMiniCard.tsx — StaticTrackMap integriert
**Suche nach:** lineStringToSvgPoints
**Delta:** Verwendet jetzt `StaticTrackMap` statt reinem SVG. Behebt Clipping-Bug (war: SVG viewBox 0 0 200 120 bei 200×200-Koordinatenraum). `lineStringToSvgPoints` nicht mehr importiert.

## CHANGELOG.md
**Delta:**
- feat(photos): Aktivitäts-Vorschaukarten in `/photos` zeigen jetzt OSM-Kartenhintergrund statt einfarbigem Hintergrund (neue `StaticTrackMap`-Komponente, Web-Mercator, kein MapLibre nötig)
- fix(photos): Track-Clipping in Aktivitäts-MiniCard behoben
