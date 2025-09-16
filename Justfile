set dotenv-filename := '${PWD}/.envrc'
set dotenv-load := true

alias b := build
alias d := dev
alias t := test-all
alias f := format-all
alias l := lint-all

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
    cd web && bun install

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
test-all: test-go test-web

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
    just build fleets
    just build fleetctl

# Run Go unit tests
test-go:
    go test -v ./...

# Run Go integration tests
test-go-integration:
    INTEGRATION=1 FLEETD_INTEGRATION_TESTS=1 go test -v ./test/integration/... ./test/e2e/...

# Run specific Go test by name
test-go-run target:
    go test -v ./... -run {{target}}

# Generate Go test coverage
test-go-coverage:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Format Go code
format-go:
    go fmt ./...

# Lint Go code
lint-go:
    go vet ./...

# Run Go backend development server
server-dev:
    go run cmd/fleets/main.go server --port 8080

# Watch and run Go backend
server-watch:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" \
    gow -e=go,proto,sql -c run cmd/fleets/main.go server --port 8080

# Web Frontend Commands

# Install web dependencies
web-install:
    cd web && bun install

# Run web development server
web-dev:
    cd web && bun dev

# Build web for production
build-web:
    cd web && bun run build

# Start production web server
web-start:
    cd web && bun start

# Run web tests
test-web:
    cd web && bun run test

# Type check TypeScript
test-web-types:
    cd web && bun run typecheck

# Format web code with Biome
format-web:
    cd web && bun run format

# Lint web code with Biome
lint-web:
    cd web && bun run lint

# Analyze web bundle size
web-analyze:
    cd web && ANALYZE=true bun run build

# Proto & Code Generation

# Generate Go and TypeScript code from proto files
proto:
    buf generate
    @echo "Generated Go code in gen/"
    @echo "Generated TypeScript code in web/lib/api/gen/"

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

# Build Docker image for backend
docker-build tag="latest":
    docker build -t fleetd/fleets:{{tag}} -f docker/Dockerfile .

# Build Docker image for web
docker-build-web tag="latest":
    docker build -t fleetd/web:{{tag}} -f web/Dockerfile ./web


# Database Commands

# Run database migrations
db-migrate:
    go run cmd/fleets/main.go migrate up

# Rollback database migration
db-rollback:
    go run cmd/fleets/main.go migrate down

# Create new migration
db-migration name:
    go run cmd/fleets/main.go migrate create {{name}}

# Reset database
db-reset:
    go run cmd/fleets/main.go migrate reset

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
    cd web && bun update

# Run security audit
audit:
    go list -json -m all | nancy sleuth
    cd web && bun audit

# Generate API documentation
docs-api:
    buf export . --output=docs/api
    echo "API documentation generated in docs/api/"

# Start documentation server
docs-serve:
    cd docs && python -m http.server 8000

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
