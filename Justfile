set dotenv-filename := '${PWD}/.envrc'
set dotenv-load := true

alias b := build

version := `git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev"`
commit_sha := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_time := `date -u '+%Y-%m-%d\\ %H:%M:%S'`
executable_extension := if os() == "windows" { ".exe" } else { "" }

# Determine the correct linker flags based on the OS
linker_flags := if os() == "linux" {
    "-extldflags '-Wl,--allow-multiple-definition'"
} else {
    ""
}

build PROGRAM:
    CGO_ENABLED=1 go build -v -ldflags "-X fleetd.sh/internal/version.Version={{version}} -X fleetd.sh/internal/version.CommitSHA={{commit_sha}} -X 'fleetd.sh/internal/version.BuildTime={{build_time}}' {{linker_flags}}" -o bin/{{PROGRAM}}{{executable_extension}} cmd/{{PROGRAM}}/main.go

build-all:
    just build fleetd &
    just build columbus &
    just build auth-server &
    just build device-server &
    just build metrics-server &
    just build update-server &
    just build storage-server

test-all:
    go test -v ./...

test-package PACKAGE:
    go test -v ./{{PACKAGE}}

test TEST:
    go test -v ./... -run {{TEST}}

format:
    go fmt ./...

run: build-all
    sleep 1  # Add a small delay
    trap 'kill $(jobs -p)' INT TERM
    ./bin/fleetd &
    LISTEN_ADDR=localhost:8081 ./bin/auth-server &
    LISTEN_ADDR=localhost:8082 ./bin/columbus &
    LISTEN_ADDR=localhost:8083 ./bin/device-server &
    LISTEN_ADDR=localhost:8084 ./bin/metrics-server &
    LISTEN_ADDR=localhost:8085 ./bin/update-server &
    LISTEN_ADDR=localhost:8086 ./bin/storage-server &
    wait

watch PROGRAM:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" gow -e=go,proto,sql -c run cmd/{{PROGRAM}}/main.go

watch-all:
    trap 'kill $(jobs -p)' INT TERM
    just watch fleetd &
    LISTEN_ADDR=localhost:8081 just watch auth-server &
    LISTEN_ADDR=localhost:8082 just watch columbus &
    LISTEN_ADDR=localhost:8083 just watch device-server &
    LISTEN_ADDR=localhost:8084 just watch metrics-server &
    LISTEN_ADDR=localhost:8085 just watch update-server &
    LISTEN_ADDR=localhost:8086 just watch storage-server &
    wait
