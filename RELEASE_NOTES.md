# Release notes

## Unreleased

### Breaking change in project configuration

Glock validates local and remote project configuration against a single
schema. If you used an earlier v2 draft, remove the top-level `version` field
and rename any source directory field named `out` to `path`.

The CLI rejects v1 configurations and the built-in `glock config migrate`
command has been removed. Before upgrading, keep a copy of your old
`glock.yml` and rewrite it using the [v2 example](glock.example.yml). Move
repository definitions into `sources` and define the commands you want to run
under `components`. Review paths and lifecycle hooks, then run
`glock config validate` before starting the project.
