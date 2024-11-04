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

build target:
    #!/usr/bin/env sh
    CGO_ENABLED={{env('CGO_ENABLED', '1')}} \
    CC={{env('CC', 'gcc')}} \
    go build -v \
    -ldflags "-X fleetd.sh/internal/version.Version={{version}} \
              -X fleetd.sh/internal/version.CommitSHA={{commit_sha}} \
              -X 'fleetd.sh/internal/version.BuildTime={{build_time}}' \
              {{linker_flags}}" \
    -o bin/{{target}}{{executable_extension}} cmd/{{target}}/main.go

build-all:
    just build fleetd &
    just build columbus &
    just build server

test-all:
    CGO_ENABLED=1 go test -v ./...

test-package target:
    CGO_ENABLED=1 go test -v ./{{target}}

test target:
    CGO_ENABLED=1 go test -v ./... -run {{target}}

format:
    go fmt ./...

run: build-all
    sleep 1  # Add a small delay
    trap 'kill $(jobs -p)' INT TERM
    ./bin/fleetd &
    LISTEN_ADDR=localhost:50051 ./bin/server &
    LISTEN_ADDR=localhost:50052 ./bin/columbus &

    wait

watch target:
    VERSION={{version}} COMMIT_SHA={{commit_sha}} BUILD_TIME="{{build_time}}" gow -e=go,proto,sql -c run cmd/{{target}}/main.go

watch-all:
    trap 'kill $(jobs -p)' INT TERM
    just watch fleetd &
    LISTEN_ADDR=localhost:50051 just watch server &
    LISTEN_ADDR=localhost:50052 just watch columbus &
    wait
