# Skadia

Skadia is a self-hosted activity, health, routing, and training-data system.
It is intended for operators who want to keep their activity files, media, maps,
and derived analytics under their own control.

> **Status:** Pre-release. The repository is being prepared for an initial
> public release. Review the configuration before using it for real data.

## What It Includes

- Activity import for FIT, GPX, and TCX files.
- Optional OAuth-based activity importers.
- Health-data ingestion for sleep, HRV, heart rate, SpO2, weight, and blood
  pressure where the source data contains those records.
- Activity lists, maps, detail views, charts, gear assignments, tags, and
  administrative maintenance screens.
- Local route calculation and local vector-map serving when the required map
  and routing data has been imported.
- Privacy-zone masking for activity coordinates returned by the API.
- Data export and import paths for user-controlled transfers.
- Optional machine-learning services for media and document workflows.

## Runtime Requirements

Skadia is not a 4 GiB RAM application in the current Compose setup.

The default Compose configuration already sets these upper bounds:

- PostgreSQL container: `POSTGRES_MEM_LIMIT=8g`
- PostgreSQL shared buffers: `POSTGRES_SHARED_BUFFERS=4GB`
- API container: `mem_limit: 16g`
- Routing service: Java heap capped at `512m`
- Optional ML and LLM profiles add separate services

Recommended host sizing:

| Installation shape | Recommended RAM | Notes |
| --- | ---: | --- |
| Development or small evaluation without ML/LLM | 16 GiB minimum | Keep imports and map data small. |
| Default local-data setup | 32 GiB | Suitable for normal self-hosted testing with local routing and map data. |
| ML or LLM profiles enabled | 32 GiB or more | CPU-only ML works, but model choice and image volume affect memory use. |
| Larger imports or multi-user deployments | Size from measured load | Tune PostgreSQL, workers, storage, and routing data per deployment. |

Storage depends on imported activities, media, map data, routing data, and
exports. Start with at least 50 GB free for a local evaluation. Larger regions
and media libraries require more.

## Prerequisites

- Linux host.
- OCI-compatible container runtime with Compose support. The tested path is
  rootless Podman with `podman compose`.
- Git, OpenSSL, and standard POSIX shell tooling.
- A filesystem layout that can store PostgreSQL data safely.

On btrfs, PostgreSQL data must use `chattr +C` on `data/postgres/` before the
database is initialized. `scripts/setup.sh` handles this for the default local
data path.

## Quick Start

```bash
git clone https://github.com/skadia-app/skadia.git
cd skadia
./scripts/setup.sh
podman compose up -d --build --remove-orphans
```

Open `http://localhost:8080`.

The first account registration becomes the administrator account unless a
bootstrap administrator is configured in the environment.

## Configuration

`scripts/setup.sh` creates local directories, writes `.env`, and generates
secrets for a Compose installation.

Important settings:

- `SKADIA_DATA_ROOT` controls where runtime data is stored. Default: `./data`.
- `SKADIA_APP_PORT` controls the local HTTP port. Default: `8080`.
- `COMPOSE_PROFILES` enables optional service groups such as `full-local-data`,
  `s3`, `ml`, `llm`, or `proxy`.
- `STORAGE_BACKEND` selects `local` or `s3` storage for library objects.
- `REGISTRATION_MODE` controls whether registration is invite-only, open, or
  approval-based.
- `POSTGRES_*` settings tune database memory, checkpoint, and I/O behavior.

Use `.env.example` as the reference for available variables. Do not copy
development passwords or generated secrets into a shared deployment.

## Optional Profiles

The base Compose stack runs the API, database, event broker, and routing
support services. Optional profiles add more services:

- `full-local-data`: local map and routing data services.
- `s3`: local S3-compatible object storage for deployments that need it.
- `ml`: machine-learning worker service.
- `llm`: local LLM broker services.
- `proxy`: reverse proxy and TLS entrypoint.

Enable only the profiles needed for the installation.

## Local Map And Routing Data

Route calculation and local maps require imported map/routing data for the
selected region. The setup script can prepare the directory layout, but it does
not make a large region import cheap. Import size, preprocessing time, and RAM
usage scale with the selected region.

For a manual local import, use:

```bash
./scripts/import-osm.sh germany
```

Use a smaller region for initial tests.

## Development

Frontend development:

```bash
cd frontend
npm install
npm run dev
```

The development server listens on `http://localhost:5173`.

Backend development normally runs through Compose:

```bash
cp docker-compose.override.example.yml docker-compose.override.yml
podman compose up -d --build api
```

Run focused backend tests from `services/api`:

```bash
go test ./internal/garmin ./internal/fitimport
```

## Backups And Data Safety

Back up at least these paths before upgrades:

- PostgreSQL data under `${SKADIA_DATA_ROOT}/postgres`
- Library data under `${SKADIA_DATA_ROOT}/library`
- Export/import artifact storage if it is not backed by S3
- `.env` and any external secret files

Schema migrations run automatically on startup through the API service.
Rollback requires an operator decision. Down-migration files are documentation
for manual recovery; they are not executed automatically by the migration
runner.

## Limitations

- The public documentation set is still being rebuilt for the initial release.
- The default Compose tuning favors a capable self-hosted machine, not a
  low-memory VPS.
- Local map and routing imports can be large and slow.
- Optional ML and LLM profiles depend heavily on model choice and host
  resources.
- Multi-process and Kubernetes-style deployments require shared storage,
  explicit worker sizing, and deployment-specific configuration review.

## License

Skadia is licensed under the GNU Affero General Public License v3.0. See
[LICENSE](LICENSE).
