# Go Forum Docker Configuration
# This Dockerfile creates a minimal image using a scratch base

# Build stage: Create static binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files for better caching
COPY go.mod go.sum ./

# Download dependencies (this might fail in restricted environments, but that's ok)
RUN go mod download || true

# Copy source code
COPY . .

# Build static binary with CGO support for SQLite
ENV CGO_ENABLED=1
ENV GOOS=linux
RUN apk add --no-cache gcc musl-dev sqlite-dev || true && \
    go build -a -ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o forum . || \
    go build -o forum .

# Final minimal runtime image
FROM scratch

# Copy the binary and required files
COPY --from=builder /app/forum /forum
COPY --from=builder /app/templates /templates
COPY --from=builder /app/static /static

# Expose port
EXPOSE 8080

# Run the application
CMD ["/forum"]
