## Project Documentation

The project documentation lives in `docs/` and is structured for agent-team-based development:

| Document | Scope | Agent Team |
|---|---|---|
| `00_OVERVIEW.md` | Architecture, team structure, cross-team interfaces | All |
| `01_INFRASTRUCTURE.md` | Docker, PostgreSQL, Garage, NATS, Traefik, OIDC, Backups | Infrastructure |
| `02_CORE_BACKEND.md` | Go API, FIT/GPX/TCX parsing, Shadow File System, Hotfolder | Core Backend |
| `03_FRONTEND.md` | React 19, Vite, MapLibre, ECharts, Tailwind v4, shadcn/ui | Frontend |
| `04_ML_AI_SERVICE.md` | FastAPI ML, autotagging, LLM integration, embeddings | ML/AI |
| `05_GEO_SPATIAL.md` | PostGIS, OSM, segments, heatmap, geo-queries | Geo & Spatial |
| `06_MOBILE_APP.md` | React Native, Expo SDK 55, GPS, BLE, offline-first | Mobile |
| `07_DATABASE_SCHEMA.md` | Complete PostgreSQL schema (all tables, indexes, policies) | All |
| `08_API_REFERENCE.md` | REST endpoints, WebSocket, NATS subjects, auth flow | All |
| `09_FEATURE_CATALOG.md` | All features with priorities (P1–P4) and team assignments | All |
| `10_DEVELOPMENT_PLAN.md` | Phased development plan (Phase 0–11) with dependencies | All |
| `11_BUILD_KICKOFF.md` | Step-by-step build instructions for initial working prototype | Core Backend, Infrastructure |

Original source documents are archived in `docs/archive/`.

---

## System Environment

- Host OS: Fedora 43 Kinoite (immutable, rpm-ostree)
- Runtime: Distrobox mit Ubuntu 24.04 Container
- CPU: Intel Xeon E5-1680 v2 (x79-Plattform, 8C/16T, kein AVX2)
- Mainboard: P9X79 WS
- RAM: 44 GB DDR3
- GPU: AMD Radeon Vega 56 mit 8GB HBM2 VRAM
  - Für llama.cpp: Vulkan-Backend (läuft auf HOST, nicht im Container)
  - Für Python ML: ROCm 6.x (direkt im Container via /dev/kfd)
- Kein CUDA, kein NVIDIA

## Local LLM

- llama.cpp ist als Dienst auf dem HOST installiert

## Paketmanager (im Container)

- apt (Ubuntu 24.04)
- KEIN rpm-ostree, KEIN dnf auf dem Host anfassen
- pip immer in virtualenv oder mit --user
