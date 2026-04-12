include .env

.PHONY: test build install run clean compile release

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --exact-match 2>/dev/null || echo dev))
LD_FLAGS := -X github.com/AzraelSec/glock/cmd/cli/commands.Version=$(VERSION)
RELEASE_TAG := $(if $(filter v%,$(VERSION)),$(VERSION),v$(VERSION))

test:
	go test ./...

build:
	go build -ldflags "$(LD_FLAGS)" -o glock ./cmd/cli

install: build
	cp glock ~/.local/bin/glock

run:
	go run -ldflags "$(LD_FLAGS)" ./cmd/cli

clean:
	rm -f glock

compile:
	echo "Compiling for every OS and Platform"
	GOOS=linux GOARCH=arm go build -ldflags "$(LD_FLAGS)" -o bin/glock-linux-arm ./cmd/cli
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LD_FLAGS)" -o bin/glock-linux-arm64 ./cmd/cli

release:
	@test -n "$(VERSION)" || (printf "Usage: make release VERSION=0.1.0\n" >&2; exit 1)
	@test "$$(git rev-parse --abbrev-ref HEAD)" = "main" || (printf "Release must run from main\n" >&2; exit 1)
	@test -z "$$(git status --porcelain)" || (printf "Working tree must be clean\n" >&2; exit 1)
	@git fetch origin main --tags
	@git pull --ff-only origin main
	@if git rev-parse "$(RELEASE_TAG)" >/dev/null 2>&1; then printf "Tag %s already exists\n" "$(RELEASE_TAG)" >&2; exit 1; fi
	@if git ls-remote --exit-code --tags origin "refs/tags/$(RELEASE_TAG)" >/dev/null 2>&1; then printf "Remote tag %s already exists\n" "$(RELEASE_TAG)" >&2; exit 1; fi
	@go test ./...
	@git tag "$(RELEASE_TAG)"
	@git push origin main "$(RELEASE_TAG)"
	@goreleaser release --clean
