FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

FROM debian:trixie-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata osm2pgsql poppler-utils ffmpeg \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /api /api
COPY internal/migrate/migrations /migrations

EXPOSE 8080

CMD ["/api"]
