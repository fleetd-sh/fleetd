# GoReleaser compatible Dockerfile for platform-api
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 -S fleetd && \
    adduser -u 1000 -S fleetd -G fleetd

WORKDIR /app

# Copy pre-built binary from GoReleaser
COPY platform-api /app/
COPY internal/database/migrations /app/internal/database/migrations

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