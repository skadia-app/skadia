FROM golang:1.26-alpine AS builder

WORKDIR /app

# Layer 1: Dependencies (cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Layer 2: Embedded resources (cached unless migrations/lua change)
COPY internal/migrate/migrations/ internal/migrate/migrations/
COPY internal/osm/skadia-osm.lua internal/osm/skadia-osm.lua

# Layer 3: All source code (rebuilt on any .go file change)
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /api ./cmd/api

FROM debian:trixie-slim

# OCI image labels — make the published container traceable back to source,
# license, and project. Standard set per OCI image spec; CI builds with
# docker/metadata-action can override `revision` and `created` with the
# build-time values without touching this file.
LABEL org.opencontainers.image.title="Skadia API"
LABEL org.opencontainers.image.description="Skadia backend — Go API + routing + parsing for FIT/GPX/TCX activity files"
LABEL org.opencontainers.image.licenses="AGPL-3.0-or-later"
LABEL org.opencontainers.image.source="https://github.com/skadia-app/skadia"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata osm2pgsql postgis poppler-utils ffmpeg \
    osmium-tool default-jre-headless gdal-bin curl \
    && rm -rf /var/lib/apt/lists/*

# BRouter mapcreator JAR for building rd5 segments from PBF files.
# Only the JAR is baked in; lookups.dat + profiles come via bind-mount
# (data/brouter/profiles) so API and BRouter containers share the same version.
COPY --from=ghcr.io/abrensch/brouter:1.7.5 /brouter.jar /brouter/brouter.jar

COPY --from=builder /api /api

EXPOSE 8080

# Liveness probe: orchestrators (Compose, k8s, Podman) restart the
# container when this fails. /healthz is the cheapest server-up check
# (no DB roundtrip); /readyz can be probed separately for readiness.
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD curl -fsS --max-time 2 http://localhost:8080/healthz || exit 1

RUN groupadd -r skadia && useradd -r -g skadia -d /nonexistent -s /usr/sbin/nologin skadia
USER skadia

CMD ["/api"]
