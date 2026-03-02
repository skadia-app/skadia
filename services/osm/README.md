# OSM-Import für Skadia

## Voraussetzungen

- osm2pgsql >= 1.7 mit Flex-Unterstützung
- PostgreSQL mit PostGIS

## Schnellstart

```bash
# Bayern importieren (Standard)
./import.sh

# Eigene PBF-Datei
./import.sh /pfad/zur/datei.osm.pbf

# Umgebungsvariablen
export SKADIA_DB=skadia
export SKADIA_DB_HOST=localhost
export SKADIA_DB_PORT=5432
export SKADIA_DB_USER=postgres
./import.sh
```

## Tabellen

| Tabelle | Inhalt |
|---------|--------|
| osm_boundaries | Admin-Grenzen (Level 2,4,6,8) |
| osm_peaks | Gipfel und Vulkane |
| osm_forests | Wälder |
| osm_waters | Seen und Gewässer |
| osm_waterways | Flüsse, Bäche, Kanäle |
| osm_settlements | Orte (Stadt bis Weiler) |
| osm_amenities | POIs (Tourismus, Gastronomie) |
