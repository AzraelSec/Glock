# AGENTS.md

## Entry Points
- CLI entrypoint is `cmd/cli/main.go`; it just calls `commands.ExecuteRoot()`.
- User-facing subcommands live under `cmd/cli/commands/`.
- Shared terminal rendering lives in `internal/ui`. Git and batch helpers are in
  `internal/git` and `internal/runner`.

## Verified Commands
- Build everything: `go build ./...`
- Build the shipped binary: `make build` or `go build -o glock cmd/cli/main.go`
- Run the CLI locally: `go run ./cmd/cli --help`
- Single package test: `go test ./internal/config`
- Single test: `go test ./internal/config -run TestRepositoryCatalog`
- Static analysis: `go tool staticcheck ./...`
- CUE tooling: `go tool cue`

## Verification Baseline
- CI runs `go fix ./...` and checks for a clean diff, then runs
  `go build -v ./...` and `go test -v -race -covermode atomic -coverprofile=covprofile ./...`.

## Config Resolution
- Config filename is hardcoded to `glock.yml`.
- Config discovery is: `GLOCK_CONFIG_PATH` if set, otherwise search upward from cwd for the nearest `glock.yml`, up to 20 parent directories.
- CUE validates local and remote configs against `#Config` in
  `internal/config/schema.cue`. The schema is unversioned and uses `sources`
  and `components`.
- Update `glock.example.yml` and its comments in the same change whenever
  config fields, defaults, validation rules, or runtime behavior make the example
  incomplete or inaccurate. Validate it with
  `GLOCK_CONFIG_PATH="$PWD/glock.example.yml" go run ./cmd/cli config validate`
  and `go test ./internal/config -run TestParseConfigV2_ValidExample`.
- The config directory is the project root; every source requires `path`. Remote paths are relative to the root; filesystem paths may be absolute or relative and refer to an existing directory. Git paths cannot lexically be the root; symlinks can resolve outside it.
- Git commands operate once per `github` source. The `head` field gives the
  configured branch or tag.
- `glock config validate` runs the real CUE and reference-validation pipeline without starting processes.

## Runtime and Lifecycle
- `start` uses engine sessions, awaited hooks, and component dotenv files; host environment values win.
- `start --repos repoA --repos repoB` filters components by Git-source key.
- No filter starts all components, including sourceless services. Glock cleans up failed components while healthy ones keep running.
- Existing startup/shutdown lists retain their behavior; `on.steps` pairs successful startup tasks with reverse-order cleanup.
- Live output queues and snapshot history are bounded and report output loss; environment values are excluded from frontend snapshots.

## Git Commands
- `status` groups repositories by working-tree and upstream status.
- `switch` opens a searchable branch picker and shows an impact review before
  switching. `switch <branch>` runs directly. `switch --force` allows dirty
  checkouts.
- `reset --exclude` filters Git source keys before checking working trees.
  By default, a dirty selected repo stops the reset before any checkout.
  `reset --skip-dirty` skips switching and pulling dirty repos and resets the
  clean ones. Tag-pinned sources are checked out in detached HEAD and skip the
  pull.
- `tag` requires `tag_pattern`; repos with `exclude_tag: true` are skipped.
- `tag --preview` resolves the per-repo tag plan using read-only local checks.
  `tag --skip-push` creates local tags.
  Push mode requires exactly one remote per repository.

## Update Command
- `update` is TUI-first by default.
- In a terminal, completed updates with failures stay open for inspection.
  Use the arrow keys to select a failure and Enter to expand it.
- For scripting/debugging, use `glock update --disableTui`; `--output` only works together with `--disableTui`.
- Updater inference is lockfile-based in this order: `yarn.lock`, `package-lock.json`, `pnpm-lock.yaml`, `Gemfile.lock`/`Gemfile`, `go.mod`, `Cargo.lock`/`Cargo.toml`.
- Per-Git-source `updater` can force a backend; `_ignore_` skips updates for that repo.

## Init Command
- `glock init local` clones repos from the resolved local `glock.yml`.
- `glock init remote <url> [dir]` downloads the remote config into `<dir>/glock.yml` before cloning.
- After a partial init, fix the reported error and rerun `glock init local`
  with the downloaded config. Existing checkouts must pass verification before
  init skips them.

## Architecture

- Engine sessions and stack execution live in `internal/engine`. The frontend
  contract lives in `internal/runtime/protocol.go`, with a client in `internal/frontend`.
- Lifecycle ordering and cleanup live in `internal/orchestrator`. Process
  management and output draining live in `internal/process`.

## Releases

- The release target runs from `main` and uses GoReleaser. Publish
  release-specific notes in the matching GitHub release description.
- For the first release with the unversioned configuration schema, include the
  breaking changes and upgrade steps from the README.
