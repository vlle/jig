---
name: setup
description: Set up jig in a workspace — create jig.yml, register the scripts that already exist and leave `jig doctor` clean. Use when the user asks to set up, initialise or adopt jig, or when jig says there is no jig.yml.
---

Usage: `/jig:setup [directory]`

The goal is a workspace where `jig ls` finds every tool and `jig doctor` exits 0, without
registering files that are not tools.

## Step 1 — pick the workspace

Use the directory from the arguments. Otherwise ask the user, and offer the root of the
current git repository, or the directory that holds several repositories if they keep
them side by side. If `jig.yml` already exists there or above, say so and go to step 3.

## Step 2 — create it

```bash
jig init <dir>
```

It writes `jig.yml` and `.jig/registry/` and reports the scripts and make, just, npm and
task targets it found. They are searchable right away; targets never need a manifest.
If the workspace has environment names such as staging and production, ask for them and
set `envs` and `prod_targets` in `jig.yml`.

## Step 3 — adopt the scripts that matter

```bash
jig doctor
```

For every "script without manifest", read the script and propose a manifest: `summary`
(the problem it answers, one line), `why`, `safety` (`read-only`, `writes` or
`destructive`, honestly), `targets`, `tags`. When there are more than ten, ask which
directories matter first instead of going through all of them. Register what the user
approves:

```bash
jig add <path> --summary "..." --why "..." --safety read-only --targets a,b --tags a,b
```

Files that are not tools (libraries, fixtures, generated code) go into `.jigignore` next
to them, one glob per line. Then `jig index` writes TOOLS.md, and `jig doctor` must exit 0;
fix what it still reports.

## Step 4 — report

Say where the workspace is, how many tools are registered, how many scripts and targets
stay found-only, and how to undo it: delete `jig.yml`, `.jig/` and `TOOLS.md`.
