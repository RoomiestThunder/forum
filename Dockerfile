FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies for CGO and SQLite
RUN apk add --no-cache build-base sqlite-dev

COPY . .

# Build stage: compile the application
ENV CGO_ENABLED=1
RUN go build -o forum \
    -ldflags="-s -w" \
    -a -installsuffix cgo

# Runtime stage: minimal production image
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies only
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

# Create a non-root user for security
RUN addgroup -g 1000 appgroup && \
    adduser -D -u 1000 -G appgroup appuser

# Copy the compiled binary from builder
COPY --from=builder /app/forum .

# Copy templates and static files
COPY templates/ ./templates/
COPY static/ ./static/

# Set proper permissions
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose application port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080 || exit 1

# Run the application
CMD ["./forum"]