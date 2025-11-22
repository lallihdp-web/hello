.PHONY: build run test clean docker-build docker-up docker-down lint fmt deps

# Build variables
BINARY_NAME=ndc-clickhouse
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/ndc-clickhouse

# Run the connector locally
run: build
	./bin/$(BINARY_NAME) serve --configuration=./config

# Run tests
test:
	$(GOTEST) -v -race ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	$(GOFMT) -s -w .

# Run linter
lint:
	golangci-lint run ./...

# Build Docker image
docker-build:
	docker build -t ndc-clickhouse:$(VERSION) .

# Start all services with Docker Compose
docker-up:
	docker-compose up -d

# Start full stack (including Hasura)
docker-up-full:
	docker-compose --profile full up -d

# Stop all services
docker-down:
	docker-compose --profile full down

# View logs
docker-logs:
	docker-compose logs -f ndc-clickhouse

# Introspect schema and generate config
introspect:
	./bin/$(BINARY_NAME) introspect --output=./config

# Generate NDC schema types
generate:
	go generate ./...

# Development mode - watch for changes and rebuild
dev:
	@echo "Starting development mode..."
	@which air > /dev/null || go install github.com/cosmtrek/air@latest
	air

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build the connector binary"
	@echo "  run            - Build and run the connector"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  fmt            - Format Go code"
	@echo "  lint           - Run linter"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-up      - Start services with Docker Compose"
	@echo "  docker-up-full - Start full stack including Hasura"
	@echo "  docker-down    - Stop all services"
	@echo "  docker-logs    - View connector logs"
	@echo "  dev            - Run in development mode with hot reload"
