set dotenv-filename := '${PWD}/.envrc'
set dotenv-load := true

alias b := build

version := `git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev"`
commit_sha := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_time := `date -u '+%Y-%m-%d\\ %H:%M:%S'`
target_os := env_var_or_default("GOOS", os())
executable_extension := if target_os == "windows" { ".exe" } else { "" }

# Determine the correct linker flags based on the target OS
linker_flags := if target_os == "linux" {
    "-extldflags '-Wl,--allow-multiple-definition'"
} else if target_os == "windows" {
    "-extldflags '-L/usr/x86_64-w64-mingw32/lib'"
} else {
    ""
}

# Build a target binary with optional architecture
# Usage: just build fleetd [arch]
# Examples:
#   just build fleetd        # builds for current arch
#   just build fleetd arm64  # builds for arm64
#   just build fleetd amd64  # builds for amd64
build target arch="":
    #!/usr/bin/env sh
    set -e

    # Set architecture-specific variables
    if [ "{{arch}}" = "" ]; then
        # Default to current architecture
        OUTPUT_NAME="{{target}}"
        EXTRA_ENV=""
    else
        # Cross-compile for specified architecture
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

# Build all targets for current architecture
build-all:
    just build fleetd
    just build fleetp
    just build fleets

# Run unit tests only (excludes integration tests)
test:
    go test -v ./...

# Run all tests including integration tests
test-all:
    FLEETD_INTEGRATION_TESTS=1 go test -v ./...

# Run integration tests only
test-integration:
    FLEETD_INTEGRATION_TESTS=1 go test -v ./test/integration/... ./test/e2e/...

# Run specific test by name
test-run target:
    go test -v ./... -run {{target}}

# Run tests with coverage
test-coverage:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

format:
    go fmt ./...

lint:
    go vet ./...
    buf lint

run: build-all
    sleep 1  # Add a small delay
    trap 'kill $(jobs -p)' INT TERM
    echo "Nothing to run"
    wait

# QEMU testing commands
setup-qemu:
    ./scripts/setup-qemu-rpi.sh

test-provision:
    ./scripts/test-provisioning.sh

create-test-image:
    ./scripts/create-test-image.sh

run-qemu image="fleetd-test.img":
    ./scripts/run-qemu-rpi.sh --image {{image}}

run-qemu-pizero2 image="fleetd-test.img":
    ./scripts/run-qemu-pizero2.sh --image {{image}}

# One-command QEMU setup: downloads, builds, and provisions everything
qemu-setup:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🚀 Setting up QEMU Raspberry Pi emulation with fleetd..."
    echo
    # Download base image and firmware
    ./scripts/setup-qemu-rpi.sh
    echo
    # Build and provision
    ./scripts/test-qemu-provisioning.sh
    echo
    echo "✅ Setup complete! Run 'just qemu-run' to start the emulator"

# Setup QEMU with Raspberry Pi OS instead of DietPi
qemu-setup-raspios:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🚀 Setting up QEMU with Raspberry Pi OS..."
    echo
    # Download Raspberry Pi OS
    ./scripts/setup-qemu-raspios.sh
    echo
    # Build fleetd
    just build fleetd arm64
    echo
    # Provision the image
    ./bin/fleetp provision \
        --device qemu-rpi/raspios-working.img \
        --wifi-ssid "TestNetwork" \
        --wifi-pass "testpass123" \
        --os raspios
    echo
    echo "✅ Setup complete! Run 'just qemu-run-raspios' to start"

# Run QEMU with Raspberry Pi OS
qemu-run-raspios:
    ./scripts/run-qemu-rpi-proper.sh --image raspios-test.img

# One-command to run the provisioned QEMU image
qemu-run:
    ./scripts/run-qemu-rpi.sh --image test-provisioned.img

# Run QEMU in headless mode (serial console only)
qemu-run-headless:
    ./scripts/run-qemu-rpi.sh --image test-provisioned.img --headless

# Check if QEMU SSH is ready
qemu-ssh-wait:
    @echo "Waiting for SSH to be ready..."
    @while ! nc -z localhost 2222 2>/dev/null; do \
        echo -n "."; \
        sleep 1; \
    done
    @echo " Ready!"
    @echo "Connect with: ssh -p 2222 root@localhost"

# Test with base DietPi image (no provisioning)
qemu-test-base:
    ./scripts/run-qemu-rpi.sh --image dietpi-expanded.img --headless

watch target:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" gow -e=go,proto,sql -c run cmd/{{target}}/main.go

watch-all:
    trap 'kill $(jobs -p)' INT TERM
    echo "Nothing to watch"
    wait
