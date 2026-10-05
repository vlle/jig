# Changelog

All notable changes to jig. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions follow [Semantic Versioning](https://semver.org/). The `--json` output of `ls`, `show`
and `doctor` and the manifest schema are the public API.

## [Unreleased]

## [1.1.0] - 2026-10-06

### Added

- `jig add <path>...` registers scripts that already exist. It derives the id from the file
  or directory name, works out `run` from the file type (an executable, `bash`, `python3`,
  `node`, `go run`), sets `workdir` when a Go tool lives in a nested module, and writes the
  manifest. `--summary`, `--why`, `--safety`, `--targets`, `--tags`, `--id` and `--run` fill
  it in one go. Nothing is written when any path fails. Without `--safety` the manifest says
  `writes`: an unknown script is not assumed harmless.
- `jig version` and `jig --version`, which work outside a workspace.
- `jig completion bash|zsh|fish` completes commands, tool ids for `run`, `show` and `src`,
  `--kind` and `--safety` values. The lookup behind TAB does not write to the run log.
- `jig ls --ids` prints only the ids, one per line.
- `jig doctor` reports `unfinished manifest` when `summary` or `why` still holds the TODO
  that `jig new` and `jig add` leave behind.
- Prebuilt binaries for Linux and macOS (amd64 and arm64) with checksums on every release.

### Changed

- `jig doctor` exits with status 1 when it finds a problem, so it can gate CI. Findings of
  level `info` do not count. The `--json` output is unchanged.
- TOOLS.md no longer carries a timestamp or the workspace directory name: the output of
  `jig index` depends only on the registry.
- The stale-index check compares content instead of modification times. A fresh clone or a
  CI checkout no longer reports a stale TOOLS.md, and a workspace with no tools and no
  TOOLS.md is clean.
- `jig new --kind go|go-parallel` outside any Go module writes a `go.mod` into the tool
  directory and runs the tool from there. Inside a module the tool joins it.
- Generated manifests quote values where YAML needs it and list the allowed `safety` values
  next to the field.

### Fixed

- `jig new` with the default kind `go` in a workspace without `go.mod` created a tool that
  `jig run` could not start (`go: cannot find main module`).

### Upgrading

Run `jig index` once after upgrading: TOOLS.md loses its timestamp line, so the first
`jig doctor` reports it as stale. Scripts that call `jig doctor` and expect exit status 0
with open problems need `|| true`.

## [1.0.0] - 2026-10-05

First public version: manifests and the registry, `ls`, `show`, `src`, `run` with the
production `--yes` guard, `new` with the go, go-parallel, bash and node scaffolds and the
animation kit, `doctor`, `index`, `init`, the home screen and tool browser, the run log,
`jig agent rules|skill` and the Claude Code `PreToolUse` hook.

[Unreleased]: https://github.com/vlle/jig/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/vlle/jig/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/vlle/jig/releases/tag/v1.0.0
