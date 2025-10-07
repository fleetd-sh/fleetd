set dotenv-filename := '${PWD}/.envrc'
set dotenv-load := true

alias b := build
alias d := dev
alias t := test-all
alias f := format-all
alias l := lint-all
alias ti := test-integration-coverage
alias tc := test-cli
alias te := test-e2e
alias td := test-docker
alias ts := test-services

version := `git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev"`
commit_sha := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_time := `date -u '+%Y-%m-%d\\ %H:%M:%S'`
target_os := env_var_or_default("GOOS", os())
executable_extension := if target_os == "windows" { ".exe" } else { "" }

linker_flags := if target_os == "linux" {
    "-extldflags '-Wl,--allow-multiple-definition'"
} else if target_os == "windows" {
    "-extldflags '-L/usr/x86_64-w64-mingw32/lib'"
} else {
    ""
}

default:
    @just --list --unsorted

# Install all dependencies (Go and Node)
install:
    go mod download
    cd studio && bun install

# Run development servers (backend + frontend)
dev:
    #!/usr/bin/env sh
    trap 'kill $(jobs -p)' INT TERM EXIT
    echo "Starting development servers..."
    just server-dev &
    just web-dev &
    wait

# Build everything for production
build-all: proto build-go build-web

# Run all tests
test-all: test test-integration test-e2e test-web

# Format all code
format-all: format-go format-web

# Lint all code
lint-all: lint-go lint-web

# Clean all build artifacts
clean:
    rm -rf bin/
    rm -rf web/.next/
    rm -rf web/out/
    rm -rf coverage/
    rm -f coverage.out coverage.html

# Go Backend Commands

# Build a Go binary with optional architecture
# Usage: just build fleetd [arch]
# Supported binaries: fleetd, device-api, platform-api, fleetctl
build target arch="":
    #!/usr/bin/env sh
    set -e

    # Set architecture-specific variables
    if [ "{{arch}}" = "" ]; then
        OUTPUT_NAME="{{target}}"
        EXTRA_ENV=""
    else
        OUTPUT_NAME="{{target}}-{{arch}}"
        export GOOS=linux
        export GOARCH={{arch}}
        EXTRA_ENV="GOOS=linux GOARCH={{arch}}"
    fi

    # Use default linker flags if not cross-compiling
    if [ "{{arch}}" = "" ]; then
        LINKER_FLAGS="{{linker_flags}}"
    else
        LINKER_FLAGS=""
    fi

    # Build the binary
    go build -v \
        -ldflags "-X fleetd.sh/internal/version.Version={{version}} \
              -X fleetd.sh/internal/version.CommitSHA={{commit_sha}} \
              -X 'fleetd.sh/internal/version.BuildTime={{build_time}}' \
              ${LINKER_FLAGS}" \
        -o bin/${OUTPUT_NAME}{{executable_extension}} cmd/{{target}}/main.go

    echo "Built: bin/${OUTPUT_NAME}{{executable_extension}}"

# Build all Go binaries
build-go:
    just build fleetd
    just build device-api
    just build platform-api
    just build fleetctl

# Build cross-platform binary
# Usage: just build-cross fleetd linux amd64
build-cross target goos goarch:
    #!/usr/bin/env sh
    set -e

    OUTPUT_NAME="{{target}}-{{goos}}-{{goarch}}"
    if [ "{{goos}}" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi

    echo "Building {{target}} for {{goos}}/{{goarch}}..."

    env GOOS={{goos}} GOARCH={{goarch}} CGO_ENABLED=0 go build -v \
        -ldflags "-X fleetd.sh/internal/version.Version={{version}} \
              -X fleetd.sh/internal/version.CommitSHA={{commit_sha}} \
              -X 'fleetd.sh/internal/version.BuildTime={{build_time}}'" \
        -o bin/${OUTPUT_NAME} cmd/{{target}}/main.go

    echo "Built: bin/${OUTPUT_NAME}"

# Build all platforms for a specific target
# Usage: just build-all-platforms fleetd
build-all-platforms target:
    #!/usr/bin/env sh
    set -e

    echo "Building {{target}} for all platforms..."

    # Linux
    just build-cross {{target}} linux amd64
    just build-cross {{target}} linux arm64
    just build-cross {{target}} linux arm

    # Windows
    just build-cross {{target}} windows amd64
    just build-cross {{target}} windows arm64

    # macOS
    just build-cross {{target}} darwin amd64
    just build-cross {{target}} darwin arm64

    echo "All platforms built for {{target}}"

# Build all targets for all platforms
build-release:
    #!/usr/bin/env sh
    set -e

    echo "Building release binaries for all platforms..."

    # Core agent binaries
    just build-all-platforms fleetd
    just build-all-platforms device-api
    just build-all-platforms platform-api
    just build-all-platforms fleetctl

    echo "Release build completed"
    ls -la bin/

# Build SDK
build-sdk:
    go build -v ./sdk/...
    @echo "SDK built successfully"

# Test SDK
test-sdk:
    go test -v ./sdk/...

# Run SDK example
sdk-example:
    go run sdk/example/main.go

# Build platform API only
build-platform:
    just build platform-api

# Build device API only
build-device:
    just build device-api

# Run unit tests (with parallel execution)
test:
    SKIP_MDNS_RACE_TEST=1 go test -v -race -p 4 -timeout 10m $(go list ./... | grep -v '/test/integration' | grep -v '/test/e2e' | grep -v '/test/performance')

# Run integration tests
test-integration: build-fleetctl
    PATH="${PWD}/bin:${PATH}" INTEGRATION=1 JWT_SECRET=test-secret-key go test -timeout 10m ./test/integration/...

# Build fleetctl binary for tests
build-fleetctl:
    @just build fleetctl

# Run performance and load tests
test-performance:
    go test -v ./test/performance/... -timeout 30m

# Run e2e tests
test-e2e: build-fleetctl
    PATH="${PWD}/bin:${PATH}" E2E=1 JWT_SECRET=test-secret-key go test -v -timeout 10m ./test/e2e/...

# Run specific test by pattern
test-run target:
    go test -v ./... -run {{target}}

# Generate test coverage report
test-coverage:
    go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Run telemetry service integration tests
test-telemetry:
    INTEGRATION=true go test -v -timeout 10m ./test/integration/telemetry_service_test.go ./test/integration/helpers_test.go

# Run settings service integration tests
test-settings:
    INTEGRATION=true go test -v -timeout 10m ./test/integration/settings_service_test.go ./test/integration/helpers_test.go


# Run CLI integration tests
test-cli:
    #!/usr/bin/env sh
    just build fleetctl
    PATH="${PWD}/bin:${PATH}" INTEGRATION=true go test -v -timeout 10m ./test/integration/cli_test.go ./test/integration/helpers_test.go


# Run all integration tests with coverage
test-integration-coverage:
    #!/usr/bin/env sh
    mkdir -p coverage
    INTEGRATION=true go test -v -timeout 20m \
        -coverprofile=coverage/integration.out \
        ./test/integration/... ./test/e2e/...
    go tool cover -html=coverage/integration.out -o coverage/integration.html
    echo "Coverage report: coverage/integration.html"

# Run tests with Docker Compose
test-docker:
    docker-compose -f test/docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner
    docker-compose -f test/docker-compose.test.yml down -v

# Start test infrastructure (PostgreSQL, Redis)
test-infra-up:
    #!/usr/bin/env sh
    echo "Starting test infrastructure..."
    docker run -d --name fleetd-test-postgres \
        -e POSTGRES_USER=fleetd_test \
        -e POSTGRES_PASSWORD=test_password \
        -e POSTGRES_DB=fleetd_test \
        -p 5433:5432 \
        postgres:17-alpine
    docker run -d --name fleetd-test-redis \
        -p 6380:6379 \
        redis:7-alpine
    echo "Waiting for services to be ready..."
    sleep 5
    echo "Test infrastructure ready!"

# Stop test infrastructure
test-infra-down:
    docker stop fleetd-test-postgres fleetd-test-redis 2>/dev/null || true
    docker rm fleetd-test-postgres fleetd-test-redis 2>/dev/null || true

# Run all tests with full coverage report
test-full-coverage:
    #!/usr/bin/env sh
    mkdir -p coverage
    echo "Running unit tests..."
    go test -v -coverprofile=coverage/unit.out ./cmd/... ./internal/... ./pkg/...
    echo "Running integration tests..."
    INTEGRATION=true go test -v -coverprofile=coverage/integration.out ./test/integration/...
    echo "Running e2e tests..."
    INTEGRATION=true go test -v -coverprofile=coverage/e2e.out ./test/e2e/...
    echo "Merging coverage reports..."
    go install github.com/wadey/gocovmerge@latest
    gocovmerge coverage/*.out > coverage/all.out
    go tool cover -html=coverage/all.out -o coverage/all.html
    go tool cover -func=coverage/all.out | tail -1
    echo "Full coverage report: coverage/all.html"

# Benchmark telemetry service
bench-telemetry:
    go test -bench=. -benchmem ./test/integration/telemetry_service_test.go ./test/integration/helpers_test.go

# Run specific integration test
test-integration-run pattern:
    INTEGRATION=true go test -v ./test/integration/... -run {{pattern}}

# Run core service tests (telemetry + settings)
test-services: test-telemetry test-settings

# Clean test artifacts
test-clean:
    rm -rf coverage/ test-results/ *.out *.html
    docker stop fleetd-test-postgres fleetd-test-redis 2>/dev/null || true
    docker rm fleetd-test-postgres fleetd-test-redis 2>/dev/null || true

# Format Go code (exclude generated files)
format-go:
    go list -e ./... 2>/dev/null | grep '^fleetd.sh/' | grep -v 'gen/google' | xargs go fmt

# Lint Go code (exclude generated files)
lint-go:
    go list -e ./... 2>/dev/null | grep '^fleetd.sh/' | grep -v 'gen/google' | xargs go vet

# Run Device API development server
device-api-dev:
    JWT_SECRET=dev-secret go run cmd/device-api/main.go --port 8080

# Run Platform API development server
platform-api-dev:
    JWT_SECRET=dev-secret go run cmd/platform-api/main.go --port 8090

# Run Platform API with REST support via Vanguard
platform-api-rest:
    JWT_SECRET=dev-secret FLEETD_ENABLE_REST=true go run cmd/platform-api/main.go --port 8090

# Run both APIs in development
server-dev:
    #!/bin/bash
    trap 'kill $(jobs -p)' INT TERM EXIT
    echo "Starting Device API and Platform API..."
    just device-api-dev &
    just platform-api-dev &
    wait

# Watch and run Device API
device-api-watch:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" \
    JWT_SECRET=dev-secret \
    gow -e=go,proto,sql -c run cmd/device-api/main.go --port 8080

# Watch and run Platform API
platform-api-watch:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" \
    JWT_SECRET=dev-secret \
    gow -e=go,proto,sql -c run cmd/platform-api/main.go --port 8090

# Web Frontend Commands

# Install web dependencies
web-install:
    cd studio && bun install

# Run web development server
web-dev:
    cd studio && bun dev

# Build web for production
build-web:
    cd studio && bun run build

# Start production web server
web-start:
    cd studio && bun start

# Run web tests
test-web:
    cd studio && bun run test

# Type check TypeScript
test-web-types:
    cd studio && bun run typecheck

# Format web code with Biome
format-web:
    cd studio && bun run format

# Lint web code with Biome
lint-web:
    cd studio && bun run lint

# Analyze web bundle size
web-analyze:
    cd studio && ANALYZE=true bun run build

# Proto & Code Generation

# Generate Go and TypeScript code from proto files
proto:
    buf generate
    # Clean up conflicting Google API packages from Go generation
    rm -rf gen/google
    @echo "Generated Go code in gen/"
    @echo "Generated TypeScript code in studio/lib/api/gen/"
    @echo "Generated OpenAPI spec in gen/docs/"

# Format proto files
proto-format:
    buf format -w

# Lint proto files
proto-lint:
    buf lint

# Breaking change detection for protos
proto-breaking:
    buf breaking --against '.git#branch=main'

# Platform Commands (managed by fleetctl)

# Start the entire platform
platform-start:
    go run cmd/fleetctl/main.go start

# Stop the platform
platform-stop:
    go run cmd/fleetctl/main.go stop

# Check platform status
platform-status:
    go run cmd/fleetctl/main.go status


# Docker Commands

# Build all Docker images for fleetctl start
docker-build-all tag="latest": (docker-build-platform-api tag) (docker-build-device-api tag) (docker-build-studio tag)
    @echo "Built all Docker images with tag: {{tag}}"

# Build Platform API Docker image
docker-build-platform-api tag="latest":
    docker build -t fleetd.sh/platform-api:{{tag}} -f Dockerfile.platform-api .

# Build Device API Docker image
docker-build-device-api tag="latest":
    docker build -t fleetd.sh/device-api:{{tag}} -f Dockerfile.device-api .

# Build Studio (web UI) Docker image
docker-build-studio tag="latest":
    docker build -t fleetd.sh/studio:{{tag}} -f studio/Dockerfile ./studio

# Build all images for fleetctl start (replaces build-go + docker builds)
build-docker tag="latest": (docker-build-all tag)
    @echo "Built all Docker images for fleetctl start with tag: {{tag}}"


# Database Commands

# Run database migrations
db-migrate:
    go run cmd/fleetctl/main.go migrate up

# Rollback database migration
db-rollback:
    go run cmd/fleetctl/main.go migrate down

# Create new migration
db-migration name:
    go run cmd/fleetctl/main.go migrate create {{name}}

# Reset database
db-reset:
    go run cmd/fleetctl/main.go migrate reset

# Deployment Commands

# Deploy to production
deploy env="production":
    #!/usr/bin/env sh
    echo "Deploying to {{env}}..."
    just build-all
    just test-all
    echo "Ready for deployment!"

# Create release
release version:
    #!/usr/bin/env sh
    git tag -a v{{version}} -m "Release v{{version}}"
    git push origin v{{version}}
    echo "Released v{{version}}"

# Utility Commands

# Check if all tools are installed
check-tools:
    #!/usr/bin/env sh
    echo "Checking required tools..."
    command -v go >/dev/null 2>&1 || { echo "go is required but not installed."; exit 1; }
    command -v bun >/dev/null 2>&1 || { echo "bun is required but not installed."; exit 1; }
    command -v buf >/dev/null 2>&1 || { echo "buf is required but not installed."; exit 1; }
    command -v docker >/dev/null 2>&1 || { echo "docker is optional but not installed."; }
    echo "All required tools are installed!"

# Update all dependencies
update-deps:
    go get -u ./...
    go mod tidy
    cd studio && bun update

# Run security audit
audit:
    go list -json -m all | nancy sleuth
    cd studio && bun audit

# Open wiki documentation
docs:
    open https://github.com/fleetd-sh/fleetd/wiki

# Generate all documentation (API + CLI)
docs-generate: docs-api docs-cli

# Generate OpenAPI documentation from protobuf definitions
docs-api:
    buf generate
    @echo "OpenAPI documentation generated at gen/docs/"

# Generate CLI documentation from Cobra commands
docs-cli:
    # Note: Using -mod=mod to bypass vendor issues during development
    go build -mod=mod -v -ldflags "-X fleetd.sh/internal/version.Version=dev" -o bin/fleetctl-temp cmd/fleetctl/main.go || echo "Build failed, trying without problematic packages"
    # If main build fails due to security package issues, generate minimal docs
    @if [ -f bin/fleetctl-temp ]; then \
        ./bin/fleetctl-temp docs --format markdown --output gen/docs/cli; \
        rm bin/fleetctl-temp; \
    else \
        echo "Could not build fleetctl due to security package compilation errors."; \
        echo "Please fix the duplicate type declarations in internal/security/ first."; \
        mkdir -p gen/docs/cli; \
        echo "# CLI Documentation" > gen/docs/cli/README.md; \
        echo "CLI documentation generation is temporarily disabled due to build issues." >> gen/docs/cli/README.md; \
        echo "Run 'fleetctl docs' after fixing the security package compilation errors." >> gen/docs/cli/README.md; \
    fi
    @echo "CLI documentation process completed at gen/docs/cli/"

# Serve OpenAPI documentation with Swagger UI
docs-serve port="8082":
    @echo "Starting Swagger UI server on http://localhost:{{port}}"
    @echo "OpenAPI spec: gen/docs/"
    go run cmd/swagger/main.go

# Generate man pages for CLI commands
docs-man:
    just build fleetctl
    ./bin/fleetctl docs --format man --output gen/docs/man
    @echo "Man pages generated at gen/docs/man/"

# Generate all documentation formats
docs-all: docs-api docs-cli docs-man
    @echo "All documentation generated in gen/docs/"

# Clean generated documentation
docs-clean:
    rm -rf gen/docs/
    @echo "Generated documentation cleaned"

# Open API documentation in browser
docs-open:
    just docs-serve &
    sleep 2
    open http://localhost:8082

# Run pre-commit checks
pre-commit: format-all lint-all test-all
    echo "All pre-commit checks passed!"

# Show project statistics
stats:
    @echo "Project Statistics"
    @echo "-------------------"
    @echo "Go files: $(find . -name '*.go' -not -path './vendor/*' | wc -l)"
    @echo "TypeScript files: $(find web -name '*.ts' -o -name '*.tsx' | wc -l)"
    @echo "Proto files: $(find proto -name '*.proto' | wc -l)"
    @echo "Total LOC: $(find . -name '*.go' -o -name '*.ts' -o -name '*.tsx' -not -path './vendor/*' -not -path './node_modules/*' | xargs wc -l | tail -1)"

# Development Helpers

# Find TODO comments in code
todos:
    @echo "TODO Comments:"
    @rg "TODO|FIXME|HACK|XXX" --type go --type ts --type tsx || true

# Start all services for local development
local:
    #!/usr/bin/env sh
    echo "Starting local development environment..."
    echo "Use 'fleetctl start --profile development' to start platform services"
    just dev

# Open project in browser
open:
    open http://localhost:3000

# Watch for file changes and run tests
watch-test:
    watchexec -e go,ts,tsx -- just test-all
