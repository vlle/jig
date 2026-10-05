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
- **Run** — `jig run <id>` from the right directory, arguments passed through untouched,
  `--yes` required for anything that writes to production.
- **Scaffold** — `jig new <id>` refuses when a similar tool already exists; otherwise it
  writes a Go, Go fan-out, bash or node skeleton with flags, timeout, dry-run, run log and
  the terminal animation kit.
- **Keep it honest** — `jig doctor` finds scripts with no manifest, manifests with no
  script, stale indexes and copy-pasted files.

Scripts stay where they are. jig only knows about them.

## Install into your agent

Paste this into Claude Code, Codex, Cursor or any agent with a shell:

````text
Install jig (https://github.com/vlle/jig), a registry for the scripts in my workspace,
and wire it into how you work. Ask before anything that writes outside the workspace.

1. Install: `go install github.com/vlle/jig@latest`, then check that `jig help` runs.
   If `jig` is not on PATH, use "$(go env GOPATH)/bin/jig" for now and tell me which
   shell rc file needs the PATH line.
2. Ask me which directory is my workspace (default: the root of the current repository)
   and run `jig init <dir>` there. If the workspace has environment names such as
   staging and production, ask me for them and set `envs` and `prod_targets` in jig.yml.
3. Run `jig doctor`. For each script it reports without a manifest, read the script and
   propose a manifest: id, path, run, summary (the problem it answers, one line), why,
   safety (read-only / writes / destructive, honestly), targets, tags. Write the ones I
   approve into the registry directory; put files that are not tools into .jigignore.
   Repeat until `jig doctor` is clean, then run `jig index`.
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

```bash
go install github.com/vlle/jig@latest
cd ~/work            # the directory that holds your repositories, or a single repo
jig init             # writes jig.yml and .jig/registry/
jig doctor           # lists the scripts that are already there without a manifest
```

`jig` finds its workspace the way git finds a repository: the nearest `jig.yml` above the
current directory. `JIG_ROOT` overrides it.

## Commands

```bash
jig                        # home screen: search, browse, doctor, recent runs
jig ls stock               # search id, summary, why, tags and path; all words must match
jig ls --tag incident      # by tag; --kind env for secret profiles, --all for everything
jig show stock-diag        # what it does, how to run it, flags, safety, targets
jig src stock-diag         # the source (through bat if it is installed)
jig run stock-diag -- -item 280982988
jig new stock-recount --kind go --dir catalog/scripts/stock-recount
jig doctor                 # problems in the registry
jig index                  # regenerate TOOLS.md
jig demo                   # the animation kit the scaffolds ship with
jig agent rules|skill      # text for your coding agent
```

`ls`, `show` and `doctor` take `--json`. That output is meant for agents and stays stable.

In the home screen, start typing to search; `enter` opens the browser, `ctrl+d` runs the
doctor, `esc` quits. In the browser: `j`/`k`, `/` search, `tab` docs ⇄ source, `enter`
run, `e` open in `$EDITOR`, `d` doctor, `y` copy the command, `esc` back home.

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
of `path` when it is not under git. `jig doctor` checks that every relative path in `run`
resolves from there.

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

## License

MIT
