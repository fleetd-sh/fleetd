# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the device-api binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s" \
    -o device-api \
    cmd/device-api/main.go

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S fleetd && \
    adduser -u 1001 -S fleetd -G fleetd

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/device-api /app/device-api

# Copy migrations if they exist
COPY --from=builder /build/internal/database/migrations /app/migrations

# Change ownership
RUN chown -R fleetd:fleetd /app

# Switch to non-root user
USER fleetd

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/device-api"]