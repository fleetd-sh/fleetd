# Multi-stage build for platform-api
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o platform-api cmd/platform-api/main.go

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S fleetd && \
    adduser -u 1000 -S fleetd -G fleetd

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/platform-api /app/
COPY --from=builder /build/internal/database/migrations /app/internal/database/migrations

# Create data directory
RUN mkdir -p /data && chown -R fleetd:fleetd /data /app

USER fleetd

# Expose port
EXPOSE 8090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/health || exit 1

# Run the binary
ENTRYPOINT ["/app/platform-api"]