FROM node:22-alpine AS frontend-builder

WORKDIR /frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
ARG SKADIA_GIT_COMMIT=unknown
ARG SKADIA_GIT_COMMIT_TIME=unknown
ARG SKADIA_BUILD_TIME=unknown
ARG SKADIA_CONFIG_VERSION=1
RUN VITE_SKADIA_GIT_COMMIT="$SKADIA_GIT_COMMIT" \
    VITE_SKADIA_GIT_COMMIT_TIME="$SKADIA_GIT_COMMIT_TIME" \
    VITE_SKADIA_BUILD_TIME="$SKADIA_BUILD_TIME" \
    VITE_SKADIA_CONFIG_VERSION="$SKADIA_CONFIG_VERSION" \
    npm run build
RUN printf '{ "service": "frontend", "git_commit": "%s", "commit_time": "%s", "build_time": "%s", "config_version": "%s" }\n' \
      "$SKADIA_GIT_COMMIT" "$SKADIA_GIT_COMMIT_TIME" "$SKADIA_BUILD_TIME" "$SKADIA_CONFIG_VERSION" \
      > /frontend/dist/skadia-build.json

FROM golang:1.26-alpine AS builder

WORKDIR /app

# Layer 1: Dependencies (cached unless go.mod/go.sum change)
COPY services/api/go.mod services/api/go.sum ./
RUN go mod download

# Layer 2: Embedded resources (cached unless migrations/lua change)
COPY services/api/internal/migrate/migrations/ internal/migrate/migrations/
COPY services/api/internal/osm/skadia-osm.lua internal/osm/skadia-osm.lua

# Layer 3: All source code (rebuilt on any .go file change)
COPY services/api/ .
ARG SKADIA_GIT_COMMIT=unknown
ARG SKADIA_GIT_COMMIT_TIME=unknown
ARG SKADIA_BUILD_TIME=unknown
ARG SKADIA_IMAGE_DIGEST=
ARG SKADIA_CONFIG_VERSION=1
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
      -ldflags "-X github.com/skadia-app/skadia/services/api/internal/buildinfo.GitCommit=${SKADIA_GIT_COMMIT} -X github.com/skadia-app/skadia/services/api/internal/buildinfo.CommitTime=${SKADIA_GIT_COMMIT_TIME} -X github.com/skadia-app/skadia/services/api/internal/buildinfo.BuildTime=${SKADIA_BUILD_TIME} -X github.com/skadia-app/skadia/services/api/internal/buildinfo.ImageDigest=${SKADIA_IMAGE_DIGEST} -X github.com/skadia-app/skadia/services/api/internal/buildinfo.ConfigVersion=${SKADIA_CONFIG_VERSION}" \
      -o /api ./cmd/api

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
COPY --from=ghcr.io/abrensch/brouter:v1.7.8 /brouter.jar /brouter/brouter.jar

COPY --from=builder /api /api
COPY --from=frontend-builder /frontend/dist /app/frontend

EXPOSE 8080

# Liveness probe: orchestrators (Compose, k8s, Podman) restart the
# container when this fails. /healthz is the cheapest server-up check
# (no DB roundtrip); /readyz can be probed separately for readiness.
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD curl -fsS --max-time 2 http://localhost:8080/healthz || exit 1

RUN groupadd -r skadia && useradd -r -g skadia -d /nonexistent -s /usr/sbin/nologin skadia
USER skadia

CMD ["/api"]
