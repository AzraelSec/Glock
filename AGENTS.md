# AGENTS.md

## Entry Points
- CLI entrypoint is `cmd/cli/main.go`; it just calls `commands.ExecuteRoot()`.
- User-facing subcommands live under `cmd/cli/commands/`.
- Shared behavior is in `internal/`, especially `internal/config`, `internal/git`, `internal/shell`, and `internal/runner`.

## Verified Commands
- Build everything: `go build ./...`
- Build the shipped binary: `make build` or `go build -o glock cmd/cli/main.go`
- Run the CLI locally: `go run ./cmd/cli --help`
- Do not use `make run`; the target still points at missing root `main.go`.
- Single package test: `go test ./internal/config`
- Single test: `go test ./internal/config -run TestRepositoryCatalog`

## Verification Baseline
- CI runs `go build -v ./...` then `go test -v -race -covermode atomic -coverprofile=covprofile ./...`.
- Current local baseline: `go build ./...` passes.
- Current local baseline: `go test ./...` passes.

## Config Resolution
- Config filename is hardcoded to `glock.yml`.
- Config discovery is: `GLOCK_CONFIG_PATH` if set, otherwise search upward from cwd for the nearest `glock.yml`, up to 20 parent directories.
- Ordinary commands require v2; v1 is read only by `glock config migrate --output <project-root>/glock.yml`; a relative v1 `root_path` requires `--v1-root <absolute-effective-root>`.
- The config directory is the project root; every source requires `path`. Remote paths are relative to the root; filesystem paths may be absolute or relative and refer to an existing directory. Git paths cannot lexically be the root; symlinks can resolve outside it.
- Git commands operate once per `github` source; `head` is its default branch.
- `glock config validate` runs the real CUE and reference-validation pipeline without starting processes.

## Runtime Quirks
- `start` uses engine sessions, awaited hooks, and component dotenv files; host environment values win.
- `start --repos repoA --repos repoB` filters components by Git-source key.
- No filter starts all components, including sourceless services. Glock cleans up failed components while healthy ones keep running.
- Existing startup/shutdown lists retain their behavior; `on.steps` pairs successful startup tasks with reverse-order cleanup.
- Live output queues and snapshot history are bounded and report output loss; environment values are excluded from frontend snapshots.
- `switch` and `reset` refuse dirty repos unless `switch --force` is used.
- `tag` requires `tag_pattern`; repos with `exclude_tag: true` are skipped.
- `tag --skip-push` avoids pushing; normal push mode fails when a repo has multiple remotes.

## Update Command
- `update` is TUI-first by default.
- For scripting/debugging, use `glock update --disableTui`; `--output` only works together with `--disableTui`.
- Updater inference is lockfile-based in this order: `yarn.lock`, `package-lock.json`, `pnpm-lock.yaml`, `Gemfile.lock`/`Gemfile`, `go.mod`, `Cargo.lock`/`Cargo.toml`.
- Per-Git-source `updater` can force a backend; `_ignore_` skips updates for that repo.

## Init Command
- `glock init local` clones repos from the resolved local `glock.yml`.
- `glock init remote <url> [dir]` downloads the remote config into `<dir>/glock.yml` before cloning.

## Architecture

See [architecture.md](architecture.md) for the engine session protocol, lifecycle, and v2 configuration.
Use `go tool cue` for CUE and `go tool staticcheck ./...` for static analysis.
