# Skadia

Selfhosted Fitness-Tracking-Plattform mit BRouter-basiertem Routing, Multi-Sport-Support, ML-gestützter Foto-Klassifizierung und vollständigem GDPR-Export. Privacy-first — deine Daten bleiben bei dir.

## Features

- **Multi-Sport-Tracking** — Run, Bike, Hike, Swim, Ski, Indoor, Strength, ... (60+ Sport-Typen)
- **Routing** — BRouter (Profile-basiert) + BadRouter (LLM-gestützt) auf Basis von OSM-Daten
- **Imports** — Garmin Connect, Strava, Komoot, FIT/GPX/TCX-Dateien, Apple Health
- **Health-Daten** — Sleep, HRV, SpO2, Weight, Blood Pressure (manuell oder via Wearable-Import)
- **Analytics** — PMC (CTL/ATL/TSB), Best Efforts, Training Load, Power Curve, Eddington
- **ML-Features** — Photo-Embedding (CLIP), Activity-Clustering, Anomalie-Erkennung
- **Privacy-Zonen** — Start-/Zieladressen automatisch maskiert
- **Multi-User** — Bis 100k+ Installationen, eigenes Auth-System (Argon2id, HIBP)

## Voraussetzungen

- **Container-Runtime:** Podman (rootless empfohlen) oder Docker
- **Compose-Plugin:** `podman compose` oder `docker compose`
- **Disk:** Min. 20 GB für OSM-Daten + Aktivitäten (mehr je nach Region)
- **RAM:** 4 GB (Basis), 8+ GB mit ML-Features
- **OS:** Linux (Fedora, Ubuntu, Debian getestet)

## Quick-Start

```bash
git clone https://github.com/skadia-app/skadia.git
cd skadia
./scripts/setup.sh           # Verzeichnisse, .env, Secrets generieren
podman compose up -d         # Stack starten
```

Browser öffnen: `http://localhost` (oder die konfigurierte Domain).

Erste Registrierung wird automatisch zum Admin-Account.

## Storage-Optionen

- **Local** (Default) — Dateien direkt im Filesystem (`./data/library/`)
- **S3** — Garage (selfhosted) oder externes S3 (AWS, Hetzner, MinIO)

Siehe `.env.example` → `STORAGE_BACKEND`.

## OSM-Import (für Routing & Karten)

Routing braucht OSM-Daten der gewünschten Region. Im Setup-Wizard wird das interaktiv eingerichtet, oder manuell:

```bash
./scripts/import-osm.sh germany   # oder dachl, europe, etc.
```

Nach dem Import: Routing-Tiles bauen via Admin-UI.

## Warnung: btrfs

Auf btrfs-Filesystemen muss `chattr +C` (nocow) auf `data/postgres/` gesetzt werden — sonst riskierst du PostgreSQL-Page-Korruption durch Copy-on-Write. Das `setup.sh` macht das automatisch. Details: `docs/operations/POSTGRES_INTEGRITY.md`.

## Konfiguration

Alle Optionen in `.env.example` dokumentiert. Wichtig:

- `DOMAIN` — Hostname (default: `localhost`)
- `STORAGE_BACKEND` — `local` oder `s3`
- `COMPOSE_PROFILES` — Optionale Features (`ml`, `llm`, `s3`, kommagetrennt)
- `REGISTRATION_MODE` — `invite`, `open` oder `approval`
- `LOG_LEVEL` — `debug`, `info`, `warn`, `error`

## Development

```bash
# Frontend (Vite Dev-Server, Hot-Reload)
cd frontend && npm run dev    # → http://localhost:5173

# Backend (im Container, mit Source-Mount via override)
cp docker-compose.override.example.yml docker-compose.override.yml
podman compose up api
```

Weitere Doku: `docs/` (Architektur, Migrations, Routing-Engine, Sport-Taxonomie).

## Lizenz

TBD
