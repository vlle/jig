---
name: jig
description: Write a script, probe or one-off tool for this workspace through the jig registry — search for an existing tool first, extend it if one fits, and scaffold a new one with a manifest only when nothing does. Use whenever the user asks for a script, a check, a diagnostic, an export or any tool that will run more than once.
---

The job of this skill is to **not write a new script when a similar one already exists**.

Usage: `/jig <what the tool should do>`

## Step 1 — search the registry (mandatory)

```bash
jig ls "<domain words>"
jig ls "<action words>"
```

Search at least twice: by the subject (`stock`, `orders`, `invoices`) and by the action
(`check`, `diag`, `export`, `stats`). `jig ls --tag <tag>` and `jig ls --all` (deprecated
tools and secret profiles too) help when the words do not match.

**Report the result to the user before doing anything else.** If `jig` is missing or
there is no `jig.yml` above the current directory, say so and stop — do not improvise.

## Step 2 — decide

- **A tool fits** → show `jig show <id>` and `jig src <id>`, propose running or extending it.
  This is the default. Create a new tool only when extending really does not work, and say why.
- **A tool is close** → show it and ask the user: extend it or create a new one.
  Do not decide for them.
- **Nothing fits** → step 3.

## Step 3 — scaffold

```bash
jig new <id> --kind go|go-parallel|bash|node --dir <dir>
```

- `go` is the default. `go-parallel` when the tool runs the same work against many targets
  (shards, hosts, regions, accounts): it ships a fan-out panel with per-target status.
  `bash` only for trivial glue around a couple of commands. `node` when the ecosystem is JS.
- Put a Go tool inside the Go module it imports from, otherwise `go run` cannot resolve it.
- Pass `--dir` explicitly; the default `scripts/<id>` is relative to the current directory.
- If `jig new` stops on "similar tools already exist", go back to step 2 with that list.

The scaffold already has fail-fast flag validation, a timeout, dry-run by default, a spinner
(or fan-out panel) that degrades to plain lines when stderr is not a TTY, progress on stderr,
results on stdout, an audit log and a run log. **Write the logic into it, do not rewrite the
plumbing.** Drop flags the tool has no use for (`-dry-run` on a read-only probe).

## Step 4 — fill the manifest

`jig new` writes `<registry>/<id>.yml`. Replace both TODOs:

- `summary` — one line in the words of the problem, not the implementation.
  Good: "published item shows out of stock in the catalogue". Bad: "items script".
- `why` — what it does and why this way; invariants and constraints go here.
- `safety` — `read-only`, `writes` or `destructive`, honestly. With a prod target this makes
  `jig run` demand `--yes`.
- `guard: self` — set by `jig new` because the scaffold starts in dry-run, so jig does not ask
  for `--yes`. Remove it if the tool writes without its own confirmation flag.
- `targets`, `tags`, `env`, `args` (every flag, with an example) — as they apply.

## Step 5 — verify

```bash
jig show <id>
jig doctor
jig index
```

`doctor` must be clean. The usual miss is "run target not found from workdir": the path in
`run` is relative to the root of the git repository the tool lives in, or to the tool's own
directory when it is not under git.

## Rules

- Secrets only through an `env` profile — never in the script, the manifest or a file on disk.
- No comments that only make sense to someone who saw this conversation; the manifest's
  `summary` and `why` carry the context.
