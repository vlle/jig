# jig

**A registry for the scripts in your workspace, so you (and your coding agent) stop
writing the same one twice.**

![jig: home screen, search, a blocked duplicate and the animation kit](docs/demo.gif)

Every team has the same graveyard: `check-stock.sh`, `check_stock2.go`, `stock-diag/`,
each written by someone who could not find the previous one. Coding agents make it
worse: a fresh session has no memory of the probe it wrote yesterday, so it writes it
again.

jig keeps a manifest for every script that is worth running twice, makes them easy to
find, runs them safely, and scaffolds new ones with the plumbing already done. It ships
with the rules, a skill and a hook that make an agent search the registry before it
writes a line.

- **Find** — `jig ls stock`, or just `jig` for the home screen with search-as-you-type.
  It works from the first minute: scripts that have no manifest yet and the targets in
  Makefile, justfile, package.json and Taskfile are searched too.
- **Run** — `jig run <id>` from the right directory, arguments passed through untouched,
  `--yes` required for anything that writes to production.
- **Scaffold** — `jig new <id>` refuses when a similar tool already exists; otherwise it
  writes a Go, Go fan-out, bash or node skeleton with flags, timeout, dry-run, run log and
  the terminal animation kit.
- **Adopt** — `jig add tools/*.sh` registers the scripts you already have and works out
  how to run each one.
- **Keep it honest** — `jig doctor` finds scripts with no manifest, manifests with no
  script, unfinished manifests, stale indexes and copy-pasted files, and exits 1 so it can
  guard CI.

Scripts stay where they are. jig only knows about them.

## Install into your agent

Paste this into Claude Code, Codex, Cursor or any agent with a shell:

````text
Install jig (https://github.com/vlle/jig), a registry for the scripts in my workspace,
and wire it into how you work. Ask before anything that writes outside the workspace.

1. Install: `go install github.com/vlle/jig@latest` (Go 1.24+), or without Go the
   prebuilt binary for this OS and CPU from https://github.com/vlle/jig/releases/latest.
   Check that `jig version` runs. If `jig` is not on PATH, use its full path for now and
   tell me which shell rc file needs the PATH line.
2. Ask me which directory is my workspace (default: the root of the current repository)
   and run `jig init <dir>` there. If the workspace has environment names such as
   staging and production, ask me for them and set `envs` and `prod_targets` in jig.yml.
3. Run `jig doctor`. For each script it reports without a manifest, read the script and
   propose: summary (the problem it answers, one line), why, safety (read-only / writes /
   destructive, honestly), targets, tags. Register the ones I approve with
   `jig add <path> --summary "..." --why "..." --safety ... --targets a,b --tags a,b`;
   put files that are not tools into .jigignore. Repeat until `jig doctor` exits 0,
   then run `jig index`.
4. Add the output of `jig agent rules` to your persistent instructions: CLAUDE.md for
   Claude Code, AGENTS.md for Codex and most others, at the workspace root unless I ask
   for the user-level file. Show me the diff before writing.
5. If you support skills, save the output of `jig agent skill` as a skill:
   ~/.claude/skills/jig/SKILL.md for Claude Code, ~/.codex/skills/jig/SKILL.md for Codex.
6. Claude Code only, and only if I say yes: merge this hook into ~/.claude/settings.json
   (or .claude/settings.json in the workspace). It bounces a brand-new script file back
   to the registry search; set JIG_HOOK=off to bypass it.
   {"hooks":{"PreToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"jig hook"}]}]}}
7. Finish with a short report: what was installed and where, how many tools are
   registered, and how to undo each step.
````

## Install by hand

With Go 1.24 or newer:

```bash
go install github.com/vlle/jig@latest
```

Or a prebuilt binary for macOS or Linux (amd64, arm64):

```bash
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -fsSL "https://github.com/vlle/jig/releases/latest/download/jig_${os}_${arch}.tar.gz" | tar -xz jig
sudo install jig /usr/local/bin/     # or any directory on PATH
jig version
```

Every release lists `checksums.txt` next to the archives. Then, in your workspace:

```bash
cd ~/work            # the directory that holds your repositories, or a single repo
jig init             # writes jig.yml and .jig/registry/, lists scripts that already exist
jig add scripts/check-stock.sh --summary "item is published but shows out of stock" --safety read-only
jig index            # TOOLS.md: the catalogue people and agents read
jig doctor           # exits 0 when the registry is in order
```

`jig` finds its workspace the way git finds a repository: the nearest `jig.yml` above the
current directory. `JIG_ROOT` overrides it.

Shell completion, tool ids for `run`, `show` and `src` included:

```bash
echo 'source <(jig completion bash)' >> ~/.bashrc
echo 'source <(jig completion zsh)' >> ~/.zshrc
jig completion fish > ~/.config/fish/completions/jig.fish
```

jig runs on macOS and Linux. `jig run` starts tools through `bash`, so on Windows use WSL.

## Commands

```bash
jig                        # home screen: search, browse, doctor, recent runs
jig ls stock               # search id, summary, why, tags and path; all words must match
jig ls --tag incident      # by tag; --kind env for secret profiles, --all for everything
jig ls --found             # only what has no manifest; --registered for the registry alone
jig show stock-diag        # what it does, how to run it, flags, safety, targets
jig src stock-diag         # the source (through bat if it is installed)
jig run stock-diag -- -item 280982988
jig new stock-recount --kind go --dir catalog/scripts/stock-recount
jig add ops/scripts/*.sh   # register scripts that already exist
jig doctor                 # problems in the registry; exit status 1 when there are any
jig index                  # regenerate TOOLS.md
jig demo                   # the animation kit the scaffolds ship with
jig agent rules|skill      # text for your coding agent
jig completion fish        # bash, zsh or fish
jig version
```

`ls`, `show` and `doctor` take `--json`. That output is meant for agents and stays stable.

In the home screen, start typing to search; `enter` opens the browser, `ctrl+d` runs the
doctor, `esc` quits. In the browser: `j`/`k`, `/` search, `tab` docs ⇄ source, `enter`
run, `e` open in `$EDITOR`, `d` doctor, `y` copy the command, `esc` back home.

## What jig finds without a manifest

`jig ls` lists the registry first, then what it found in the workspace that nobody has
registered yet:

```text
$ jig ls stock
found in the workspace, not registered
scripts/check-stock.sh   tells why a published item shows out of stock
scripts/check_stock2.py  Same question, asked from the warehouse side
```

- **Scripts** without a manifest, the same files `jig doctor` reports. The summary comes
  from the header comment, a Python docstring or `argparse` description, or the usage line.
  jig only reads the file and never runs it. A Go tool is its `package main` directory;
  `.py` files need a shebang or a `__main__` block and `.js` files a shebang or
  `process.` without `document.`, so library modules and browser code stay out.
- **Targets** of `Makefile` (`## description` or the comment above), `justfile` (the
  comment above or `[doc(...)]`, private recipes skipped), `package.json` scripts (run with
  npm, pnpm, yarn or bun, whichever lockfile is there) and `Taskfile.yml` (`desc`).

The id is the path from the workspace root (`scripts/check-stock.sh`) or the file and the
target (`Makefile:build`, `web/package.json:lint`). `jig show`, `jig src`, `jig run` and
the home screen work with these ids; `run` needs the exact id and accepts a path relative to
the current directory. Found entries carry `"registered": false` and an `origin` (`script`,
`make`, `just`, `npm`, `task`) in `--json`; `safety` is `writes` because nobody has checked.
`.jigignore` and `skip` hide them the same way they hide doctor findings.

## Adopting the scripts you already have

`jig init` and `jig doctor` list every script without a manifest. `jig add` registers them:

```bash
jig add ops/scripts/rotate-keys.sh --summary "signing key is about to expire" \
  --why "keys live 90 days; this rotates them in the vault and restarts the signers" \
  --safety destructive --targets prod --tags keys,ops
jig add tools/*.py         # several at once; summary and why stay TODO for you to fill in
```

The id comes from the file name, or from the directory for `main.go`, `run.js` and the
like; `--id` overrides it. Without `--summary` the summary comes from the script's header the
same way `jig ls` finds it. `run` is written the way jig will execute it: `./x.sh` for an
executable, otherwise `bash`, `python3`, `node` or `go run`, with `workdir` pointing at the
Go module when the tool lives in a nested one. Without `--safety` the manifest says
`writes`, because nobody has checked yet. `jig doctor` reports the manifest as unfinished
until `summary` and `why` are filled in. If one path fails, nothing is written. Files that
are not tools go into `.jigignore`.

## Manifests

One YAML file per tool in the registry (`.jig/registry/<id>.yml` by default). The script
itself stays wherever it lives.

```yaml
id: stock-diag
path: catalog/scripts/stock-diag   # directory or file, relative to the workspace
summary: item is published but the catalogue shows it out of stock
run: go run ./catalog/scripts/stock-diag
safety: read-only                  # read-only | writes | destructive
targets: [prod]
guard: self                        # the tool has its own dry-run; jig does not ask for --yes
env: pg-ro                         # id of a `kind: env` manifest that provides secrets
args:
  - flag: -item
    desc: item id
    example: "280982988"
why: |
  What it does and why this way and not another.
tags: [catalog, stock, incident]
status: active                     # active | incident-only | deprecated
ticket: OPS-123
kind: tool                         # tool | env | suite
source: cmd/main.go                # only when jig src cannot guess the entry file
workdir: .                         # override the directory run starts in
```

Required: `id`, `path`, `summary`, `run`. Unknown keys are an error, so typos surface in
`jig doctor` instead of being ignored.

`run` starts in the **root of the git repository** that holds `path`, or in the directory
of `path` when it is not under git; `workdir` is relative to that. `jig doctor` checks that
every relative path in `run` resolves from there. `jig new` and `jig add` work it out for
you.

`safety: writes` or `destructive` together with a target listed in `prod_targets` makes
`jig run` refuse without `--yes`. This is behaviour, not documentation. `guard: self`
lifts it for tools that protect themselves; `jig new` sets it because every scaffold
starts in dry-run. Remove it if you remove the dry-run.

## jig.yml

Every key is optional; `jig init` writes this file with the defaults.

```yaml
registry: .jig/registry            # where manifests live
index: TOOLS.md                    # generated catalogue, "-" disables it
skip: []                           # directories doctor never walks
tool_dirs: [scripts, tools]        # .go/.js/.mjs/.py below these count as scripts
prod_targets: [prod, prd, production]
envs: []                           # e.g. [stage, prod]: scaffolds get -env with these values
```

`skip` matches a bare name at any depth (`generated`), a path relative to the workspace
(`svc/legacy`), or an anchored top-level name (`/old-team`). `vendor`, `node_modules`,
`dist`, `build`, `target`, `__pycache__` and hidden directories are always skipped.
`.sh` files count as scripts anywhere. For single files, drop a `.jigignore` next to them
or in any parent: one glob, file name or directory prefix per line.

## Scaffolds and the animation kit

`jig new <id> --kind KIND` writes a working skeleton and its manifest:

| kind | for | you write |
|---|---|---|
| `go` | a probe or a fix with one main phase | `collect` |
| `go-parallel` | the same work against many targets: shards, hosts, regions | `defaultTargets`, `inspect` |
| `bash` | glue around a couple of commands | `collect` |
| `node` | a JS ecosystem task | `collect` |

Every scaffold validates its flags with clear messages, has an overall timeout, starts in
dry-run, prints progress to stderr and results to stdout, writes a run log line and takes
`-env` when `envs` is set in `jig.yml`.

A Go scaffold joins the Go module above it, so it can import that module's packages. With
no module between the tool and the repository root, `jig new` writes a `go.mod` into the
tool directory and `run` becomes `go run .` from there.

The Go scaffolds also get `screen.go` — the same file as [`internal/anim`](internal/anim/anim.go),
stdlib only — so a tool looks good while it works:

```go
s, _ := NewScreen(Options{Mode: "auto", LogPath: "auto", Tool: "stock-diag"})
defer s.Close()

err := s.Step("connecting", func(report func(string)) error {
	report("tls handshake")
	return connect(ctx)
})

err = s.Count("reading rows", total, func(advance func(int)) error {
	for rows.Next() { advance(1) }
	return rows.Err()
})

tasks := s.Fan(ctx, "shards", shards, 4, func(ctx context.Context, shard string, report func(string)) (string, error) {
	report("querying")
	return query(ctx, shard)
})
```

`Step` is a spinner with elapsed time, `Count` a progress bar, `Fan` a live panel with one
row per parallel task. `Bar` and `Sparkline` are there for your own lines. Animation goes
to stderr only and turns itself off when stderr is not a terminal, under `NO_COLOR` or
`TERM=dumb`: every step then prints one plain line, so logs and pipes stay clean. The
cursor comes back on Ctrl+C. The audit log (`-log`, on by default) receives every event
without escape codes. `jig demo` shows all of it.

## Agent integration

- `jig agent rules` — the instructions block for CLAUDE.md / AGENTS.md: search twice
  before writing, extend before creating, scaffold instead of an empty file, finish the
  manifest, keep `jig doctor` clean.
- `jig agent skill` — a skill (`/jig <what you need>`) that walks the same route step by step.
- `jig hook` — a Claude Code `PreToolUse` hook. When the agent is about to `Write` a new
  script into a tool directory that no manifest covers, the hook denies it with a reason
  that names the `jig ls` query to run and the `jig new` command to use. Edits, files
  outside the workspace, ignored paths and files inside a registered tool pass through.
  `JIG_HOOK=off` disables it.

## In CI

`jig doctor` exits with status 1 when it finds a problem: a broken manifest, a script
without one, an unfinished manifest, a `run` that does not resolve, or a TOOLS.md that no
longer matches the registry. TOOLS.md depends only on the manifests, so commit it and let
CI hold the line:

```yaml
- uses: actions/setup-go@v7
  with:
    go-version: stable
- run: go install github.com/vlle/jig@v1.1.0
- run: jig doctor
```

Findings of level `info`, such as a repository checked out twice, never fail the run.

## Run log

Every `jig` command appends one JSON line to `~/.local/state/jig/runs.jsonl`
(`XDG_STATE_HOME` is honoured, `JIG_LOG` overrides the path, `-` turns it off):

```json
{"ts":"2026-10-05T14:03:11+03:00","source":"jig","run_id":"dllqc3-18463","cmd":"run",
 "argv":["stock-diag","--","-item","280982988"],"tool":"stock-diag","safety":"read-only",
 "targets":["prod"],"exit_code":0,"duration_ms":8421,"tty":true}
```

Values of flags whose name looks secret (`token`, `pass`, `secret`, `key`, `cookie`,
`auth`, `cred`) are written as `***`. Tool output never reaches the log. Scaffolded tools
write their own line with `"source":"tool"` when started directly; under `jig run` they
see `JIG_RUN_ID` and stay quiet, so nothing is counted twice. The home screen shows the
latest runs from this file.

## Environment

| variable | effect |
|---|---|
| `JIG_ROOT` | workspace root instead of searching for `jig.yml` |
| `JIG_REGISTRY`, `JIG_INDEX` | override `registry` and `index` from `jig.yml` |
| `JIG_LOG` | run log path, `-` disables it |
| `JIG_ANIM` | `auto` (default), `always` or `never` for jig's own spinners |
| `JIG_HOOK` | `off` makes `jig hook` allow everything |
| `JIG_RUN_ID`, `JIG_TOOL` | set by `jig run` for the tool it starts |
| `NO_COLOR` | no colour and no animation |

## Development

```bash
go test ./...              # unit tests and the end-to-end suite (builds the binary)
golangci-lint run
vhs docs/demo.tape         # re-record docs/demo.gif
```

The end-to-end suite in `e2e/` runs the real binary against a temporary workspace for every
scenario, with the test binary itself standing in as the tool, so it sees exactly the argv,
working directory and environment a tool would get.

[CONTRIBUTING.md](CONTRIBUTING.md) has the conventions, [RELEASING.md](RELEASING.md) the
release checklist, [CHANGELOG.md](CHANGELOG.md) what changed.

## License

MIT
