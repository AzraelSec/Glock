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
- Single test: `go test ./internal/config -run TestNewConfigManagerYaml`

## Verification Baseline
- CI runs `go build -v ./...` then `go test -v -race -covermode atomic -coverprofile=covprofile ./...`.
- Current local baseline: `go build ./...` passes.
- Current local baseline: `go test ./...` passes.

## Config Resolution
- Config filename is hardcoded to `glock.yml`.
- Config discovery is: `GLOCK_CONFIG_PATH` if set, otherwise search upward from cwd for the nearest `glock.yml`, up to 20 parent directories.
- `root_path` overrides the config file directory when resolving repo paths.
- Per-repo `rel_path` overrides the default repo directory; otherwise the repo key name is used as the directory name.
- `root_main_stream` sets the default branch for all repos; per-repo `main_stream` overrides it.

## Runtime Quirks
- `start` loads env files listed in `env_filenames` for repo commands only.
- `services` commands run from the directory containing `glock.yml`, not from each repo directory.
- `start` starts configured services first, sleeps 5 seconds, then starts repo `on_start` commands.
- `start --repos repoA --repos repoB` is the repo filter.
- `switch` and `reset` refuse dirty repos unless `switch --force` is used.
- `tag` requires `tag_pattern`; repos with `exclude_tag: true` are skipped.
- `tag --skip-push` avoids pushing; normal push mode fails when a repo has multiple remotes.

## Update Command
- `update` is TUI-first by default.
- For scripting/debugging, use `glock update --disableTui`; `--output` only works together with `--disableTui`.
- Updater inference is lockfile-based in this order: `yarn.lock`, `package-lock.json`, `pnpm-lock.yaml`, `Gemfile.lock`/`Gemfile`, `go.mod`, `Cargo.lock`/`Cargo.toml`.
- Per-repo `updater` can force a backend; `_ignore_` skips updates for that repo.

## Init Command
- `glock init local` clones repos from the resolved local `glock.yml`.
- `glock init remote <url> [dir]` downloads the remote config into `<dir>/glock.yml` before cloning.
