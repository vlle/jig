# Contributing to jig

Issues and pull requests are welcome. For anything larger than a fix, open an issue first
and describe the problem it solves; jig stays small on purpose.

## Setup

```bash
git clone https://github.com/vlle/jig && cd jig
go test ./...          # unit tests and the end-to-end suite
golangci-lint run      # v2, default linters
go run . demo          # the animation kit, a quick smoke test of a build
```

Go 1.24 or newer. `golangci-lint` v2.13 is what CI runs.

## Where things live

| file | what |
|---|---|
| `main.go` | command dispatch and usage, nothing else |
| `config.go` | `jig.yml` discovery and defaults; nothing org-specific is hardcoded |
| `manifest.go`, `registry.go` | manifest schema, validation, YAML output; registry scan and git info read from `.git/` |
| `commands.go` | `ls`, `show`, `src`, `run` |
| `discover.go`, `header.go`, `runners.go` | the workspace walk; scripts and make/just/npm/task targets without a manifest, their summaries |
| `add.go`, `new.go`, `doctor.go`, `index.go`, `init.go`, `hook.go`, `agent.go`, `demo.go`, `version.go` | one command each |
| `home.go`, `tui.go` | the home screen and browser (bubbletea) |
| `internal/anim` | stdlib-only animation kit, copied verbatim into Go scaffolds as `screen.go` |
| `templates/` | scaffolds for `jig new` |
| `agent/` | the rules and skill printed by `jig agent` |
| `e2e/` | black-box tests of the built binary |

## Conventions

- Behaviour changes come with an end-to-end test in `e2e/`. The suite builds the binary,
  creates a temporary workspace per test and uses the test binary itself as the tool, so a
  test sees the exact argv, working directory and environment a tool gets.
- The `--json` output of `ls`, `show` and `doctor`, the manifest schema and the exit codes
  are the public API. Adding a field is fine; renaming or removing one is a breaking change.
- `internal/anim` imports the standard library only and must keep compiling when its
  `package anim` line is rewritten to `package main`.
- Progress and animation go to stderr, results to stdout. Animation turns itself off when
  stderr is not a terminal, under `NO_COLOR` and with `TERM=dumb`.
- Never print or log secret values; flag values that look secret become `***`.
- Errors are lowercase, wrapped with `fmt.Errorf("context: %w", err)`.
- Names carry the meaning; comments are for what the code cannot say.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `feat: ...`, `fix(doctor): ...`, `docs: ...`.
- User-visible changes get a line in the `Unreleased` section of [CHANGELOG.md](CHANGELOG.md).

## Pull requests

CI runs `go mod tidy -diff`, `go vet`, `go test -race` on Linux and macOS, `golangci-lint`
and a GoReleaser snapshot build. All of it has to pass. If the change touches what the home
screen or `jig demo` look like, re-record the GIF with `vhs docs/demo.tape`.
