#!/usr/bin/env bash
set -euo pipefail

workspace=${1:?usage: setup.sh <workspace-dir>}
rm -rf "$workspace"
mkdir -p "$workspace"
cd "$workspace"
git init -q
printf 'module example.com/shop\n\ngo 1.24\n' > go.mod

export JIG_ROOT="$workspace" JIG_LOG="$workspace/.runs.jsonl"
jig init >/dev/null 2>&1
sed -i.bak -e 's/^envs: \[\]/envs: [stage, prod]/' jig.yml && rm jig.yml.bak

scaffold() {
  jig new "$1" --kind "$2" --dir "$3" --force >/dev/null
}

scaffold stock-diag go catalog/scripts/stock-diag
scaffold orders-export go orders/scripts/orders-export
scaffold shard-health go-parallel platform/scripts/shard-health
scaffold promo-deadline-shift go promo/scripts/promo-deadline-shift
scaffold kafka-peek bash platform/scripts/kafka-peek
scaffold invoice-replay node billing/scripts/invoice-replay
mkdir -p platform/scripts/pg-ro && printf '#!/usr/bin/env bash\necho "export PG_USER=ro"\n' > platform/scripts/pg-ro/env.sh

manifest() {
  cat > ".jig/registry/$1.yml"
}

manifest stock-diag <<'YAML'
id: stock-diag
path: catalog/scripts/stock-diag
summary: item is published but the catalogue shows it out of stock
run: go run ./catalog/scripts/stock-diag
safety: read-only
targets: [prod]
env: pg-ro
args:
  - flag: -item
    desc: item id
    example: "280982988"
why: |
  Walks the item through every stage that can hide stock: listing status, quantity,
  ban flags, blocks and the last stock push. Prints the first stage that disagrees.
tags: [catalog, stock, incident]
YAML

manifest orders-export <<'YAML'
id: orders-export
path: orders/scripts/orders-export
summary: export orders for a date range to CSV
run: go run ./orders/scripts/orders-export
safety: read-only
targets: [stage, prod]
env: pg-ro
args:
  - flag: -from
    desc: first day, inclusive
    example: 2026-09-01
  - flag: -to
    desc: last day, inclusive
    example: 2026-09-30
why: |
  Finance asks for this every month-end. Reads from the replica, never the primary.
tags: [orders, export, finance]
YAML

manifest shard-health <<'YAML'
id: shard-health
path: platform/scripts/shard-health
summary: which database shards are lagging or refusing connections
run: go run ./platform/scripts/shard-health
safety: read-only
targets: [stage, prod]
env: pg-ro
why: |
  Ten shards in parallel with a live panel; one line per shard on stdout for grep.
tags: [db, shards, incident]
YAML

manifest promo-deadline-shift <<'YAML'
id: promo-deadline-shift
path: promo/scripts/promo-deadline-shift
summary: move promotion deadlines in the expiry tracker
run: go run ./promo/scripts/promo-deadline-shift
safety: writes
targets: [stage, prod]
guard: self
why: |
  Dry-run by default; -dry-run=false writes in one transaction and prints the diff.
tags: [promo, expiry]
YAML

manifest kafka-peek <<'YAML'
id: kafka-peek
path: platform/scripts/kafka-peek
summary: read a topic without moving the consumer group offsets
run: ./platform/scripts/kafka-peek/main.sh
safety: read-only
targets: [prod]
tags: [kafka, debug]
YAML

manifest invoice-replay <<'YAML'
id: invoice-replay
path: billing/scripts/invoice-replay
summary: replay failed invoice webhooks
run: node ./billing/scripts/invoice-replay/run.js
safety: writes
targets: [stage, prod]
tags: [billing, webhooks]
YAML

manifest pg-ro <<'YAML'
id: pg-ro
kind: env
path: platform/scripts/pg-ro/env.sh
summary: read-only database credentials for prod probes
run: ./platform/scripts/pg-ro/env.sh
tags: [secrets, db]
YAML

jig index >/dev/null 2>&1

now=$(date +%s)
stamp() { date -u -r "$((now - $1))" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@$((now - $1))" +%Y-%m-%dT%H:%M:%SZ; }
{
  printf '{"ts":"%s","source":"jig","cmd":"run","tool":"orders-export","exit_code":0}\n' "$(stamp 172800)"
  printf '{"ts":"%s","source":"jig","cmd":"run","tool":"shard-health","exit_code":1}\n' "$(stamp 10800)"
  printf '{"ts":"%s","source":"jig","cmd":"run","tool":"stock-diag","exit_code":0}\n' "$(stamp 1500)"
} > "$JIG_LOG"
