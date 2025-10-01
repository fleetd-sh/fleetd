# fleetd Stability Test Framework Makefile

.PHONY: build test clean install run quick-test config validate help

# Build configuration
BINARY_NAME = stability
BUILD_DIR = .
GO_FILES = $(shell find . -name "*.go" -not -path "./vendor/*")
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Default target
all: build

## Build the stability test binary
build:
	@echo "Building stability test framework..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./main.go
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

## Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

## Run unit tests
test:
	@echo "Running unit tests..."
	go test -v ./...

## Run unit tests with coverage
test-coverage:
	@echo "Running unit tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -rf stability-results/
	rm -rf ci-stability-results/

## Install the binary to $GOPATH/bin
install: build
	@echo "Installing to $GOPATH/bin..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

## Run a full 72-hour stability test
run: build
	@echo "Starting 72-hour stability test..."
	./$(BINARY_NAME) -duration 72h -output ./stability-results

## Run a quick development test (1 hour)
quick-test: build
	@echo "Starting quick stability test (1 hour)..."
	./$(BINARY_NAME) -duration 1h -output ./quick-results -components memory,connections

## Generate default configuration
config: build
	@echo "Generating configuration template..."
	./$(BINARY_NAME) -generate-config ./stability-config.json
	@echo "Configuration template created: stability-config.json"

## Validate configuration file
validate: build
	@if [ -f "./stability-config.json" ]; then \
		echo "Validating configuration..."; \
		./$(BINARY_NAME) -validate-config ./stability-config.json; \
	else \
		echo "No configuration file found. Run 'make config' first."; \
	fi

## Run CI stability test
ci-test:
	@echo "Running CI stability test..."
	../../scripts/ci-stability-test.sh --duration 10m --output ./ci-results

## Format Go code
fmt:
	@echo "Formatting Go code..."
	go fmt ./...

## Run Go linter
lint:
	@echo "Running Go linter..."
	golangci-lint run ./...

## Run Go mod tidy
tidy:
	@echo "Tidying Go modules..."
	go mod tidy

## Check for security vulnerabilities
security:
	@echo "Checking for security vulnerabilities..."
	gosec ./...

## Build for multiple platforms
build-all: clean
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./main.go
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./main.go
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./main.go
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./main.go
	@echo "Built binaries for multiple platforms"

## Create release archive
release: build-all
	@echo "Creating release archives..."
	mkdir -p release

	# Linux AMD64
	tar czf release/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz \
		$(BINARY_NAME)-linux-amd64 README.md

	# Linux ARM64
	tar czf release/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz \
		$(BINARY_NAME)-linux-arm64 README.md

	# macOS AMD64
	tar czf release/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz \
		$(BINARY_NAME)-darwin-amd64 README.md

	# macOS ARM64
	tar czf release/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz \
		$(BINARY_NAME)-darwin-arm64 README.md

	# Windows
	zip -q release/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip \
		$(BINARY_NAME)-windows-amd64.exe README.md

	@echo "Release archives created in release/"

## Development setup
dev-setup: deps
	@echo "Setting up development environment..."

	# Install development tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest

	# Create development configuration
	@if [ ! -f "./dev-config.json" ]; then \
		echo "Creating development configuration..."; \
		$(MAKE) build && ./$(BINARY_NAME) -generate-config ./dev-config.json; \
		echo "Development setup complete!"; \
		echo ""; \
		echo "Quick start:"; \
		echo "  make quick-test    # Run 1-hour test"; \
		echo "  make ci-test       # Run 10-minute CI test"; \
		echo "  make run          # Run full 72-hour test"; \
	fi

## Show benchmarks
benchmark: build
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## Generate documentation
docs:
	@echo "Generating documentation..."
	godoc -http=:6060 &
	@echo "Documentation server started at http://localhost:6060"
	@echo "Press Ctrl+C to stop"

## Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t fleetd-stability-test .

## Docker run
docker-run: docker-build
	@echo "Running stability test in Docker..."
	docker run -v $(PWD)/results:/app/results fleetd-stability-test

## Show help
help:
	@echo "fleetd Stability Test Framework"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 } /^##@/ { printf "\n%s\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""
	@echo "Examples:"
	@echo "  make build         # Build the stability test binary"
	@echo "  make quick-test    # Run a 1-hour development test"
	@echo "  make ci-test       # Run a 10-minute CI test"
	@echo "  make config        # Generate configuration template"
	@echo "  make run           # Run full 72-hour test"
	@echo ""
	@echo "For more information, see README.md"

# Default help target
.DEFAULT_GOAL := help