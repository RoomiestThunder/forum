# Go Forum Makefile

.PHONY: help build run docker-build docker-run docker-compose-up docker-compose-down clean

# Default target
help:
	@echo "Available commands:"
	@echo "  build           - Build the Go binary"
	@echo "  run             - Run the forum locally"
	@echo "  docker-build    - Build Docker image (minimal)"
	@echo "  docker-run      - Run Docker container"
	@echo "  compose-up      - Start with Docker Compose"
	@echo "  compose-down    - Stop Docker Compose"
	@echo "  compose-dev     - Start development environment"
	@echo "  clean           - Clean build artifacts"
	@echo "  test            - Run tests"

# Build the binary
build:
	CGO_ENABLED=1 go build -a -ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o forum

# Run locally
run: build
	./forum

# Build Docker image
docker-build:
	docker build -f Dockerfile.minimal -t go-forum .

# Run Docker container
docker-run: docker-build
	docker run -p 8080:8080 -v forum_data:/data -e DB_PATH=/data/forum.db go-forum

# Start with Docker Compose
compose-up:
	docker compose up

# Stop Docker Compose
compose-down:
	docker compose down

# Development environment
compose-dev:
	docker compose -f docker-compose.dev.yml up

# Clean build artifacts
clean:
	rm -f forum
	docker image prune -f

# Run tests (if any)
test:
	go test ./...