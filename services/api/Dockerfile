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

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata osm2pgsql postgis poppler-utils ffmpeg \
    osmium-tool \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /api /api

EXPOSE 8080

CMD ["/api"]
