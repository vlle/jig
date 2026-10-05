## Scripts and one-off tooling — jig

Every script in this workspace that will run more than once is registered in **jig**
(`jig.yml` at the workspace root). A script that is not registered cannot be found by
the next session, so it gets rewritten from scratch. jig exists to stop that.

**Search before you write.** Before writing any script, probe or one-off tool:

1. `jig ls <words>` with the domain word (`stock`, `orders`, `billing`) and again with the
   action word (`check`, `diag`, `export`, `stats`). Use `jig ls --tag <tag>` when you know one.
2. Tell the user what you found before doing anything else.
3. Something fits → `jig show <id>` and `jig src <id>`, then run it or extend it.
   Extending an existing tool is the default; a new one needs a reason you can state.
4. Something is close → show it and ask whether to extend it or create a new tool.
5. Nothing fits → scaffold, never start from an empty file:
   `jig new <id> --kind go|go-parallel|bash|node --dir <dir>`.
   The scaffold already has fail-fast flag validation, a timeout, dry-run by default,
   a spinner or fan-out panel that degrades to plain lines off a TTY, progress on stderr
   and results on stdout. Write the logic into it; do not rewrite that plumbing.

**Finish the manifest before handing off.** `jig new` writes `<registry>/<id>.yml` with two
TODOs. Replace both:
- `summary` — one line phrased as the problem it answers ("published item shows out of
  stock"), not as the implementation ("script for items").
- `why` — what it does and why this way; non-obvious invariants go here.
- `safety` honestly: `read-only`, `writes` or `destructive`. With a prod target in
  `targets` this makes `jig run` demand `--yes`, unless `guard: self` says the tool guards
  itself (dry-run by default). Remove `guard: self` if the tool loses its dry-run.
- `targets`, `tags`, `env` (a `kind: env` profile id for secrets), `args` for every flag.

Then `jig doctor` must be clean and `jig index` regenerates TOOLS.md.

**Never** put secrets in a script or a manifest — load them through an `env` profile.
`jig run <id>` needs the exact id; everything after `--` goes to the tool untouched.
`jig ls`, `jig show` and `jig doctor` take `--json` — that output is stable, use it.

A one-off command that will never run again (a single grep, a quick count) is not a tool
and does not need any of this.
