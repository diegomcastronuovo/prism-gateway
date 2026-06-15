# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files first for better caching
COPY go.mod go.sum ./
# Allow fetching new modules added to go.mod that may not yet be in go.sum
RUN go mod download 2>/dev/null || true

# Copy source code
COPY . .

# Build binary — -mod=mod allows go to update go.sum for newly added dependencies
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -mod=mod \
    -ldflags='-w -s -extldflags "-static"' \
    -o gateway \
    ./cmd/gateway

# Runtime stage
FROM alpine:3.19

# Install ca-certificates for HTTPS calls to LLM providers
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gateway /app/gateway

# Default config path (can be overridden)
ENV CONFIG_PATH=/app/config.yaml

# Expose HTTP port
EXPOSE 8080

# Health check endpoint
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run as non-root user
RUN adduser -D -u 10001 gateway
USER gateway

ENTRYPOINT ["/app/gateway"]
CMD ["-config", "/app/config.yaml"]
