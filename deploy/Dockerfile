# ─────────────────────────────────────────────
# Stage 1: Build
# ─────────────────────────────────────────────
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build all three binaries
COPY . .
RUN mkdir -p db/migrations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /bin/api ./cmd/api && \
    go build -ldflags="-s -w" -o /bin/worker ./cmd/worker && \
    go build -ldflags="-s -w" -o /bin/migrate ./cmd/migrations

# ─────────────────────────────────────────────
# Stage 2: Runtime
# ─────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl

# Non-root user
RUN addgroup -S cashpilotx && adduser -S cashpilotx -G cashpilotx

WORKDIR /app

COPY --from=builder /bin/api .
COPY --from=builder /bin/worker .
COPY --from=builder /bin/migrate .
COPY --from=builder /app/db/migrations ./db/migrations

USER cashpilotx

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/health || exit 1
