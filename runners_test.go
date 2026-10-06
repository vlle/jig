package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMakeTargets(t *testing.T) {
	makefile := `BIN := jig
VERSION ?= dev
URL = http://example.com:8080
export GOFLAGS := -mod=mod
.PHONY: build test lint
.DEFAULT_GOAL := build

define banner
fake: target inside define
endef

# Builds the binary.
# Second line.
build: deps ## compile the release binary
	go build -o $(BIN) .

# Runs every test.
test:
	go test ./...

lint fmt: ## static checks
	golangci-lint run

build       — not a rule
check: ## check       — fmt + vet
	true

$(BIN): main.go
	go build

%.o: %.c
	cc -c $<

dist/app: build
	cp app dist/

foo.txt:
	touch foo.txt
`
	got := makeTargets(makefile)
	want := []runner{
		{name: "build", summary: "compile the release binary", run: "make build"},
		{name: "test", summary: "Runs every test", run: "make test"},
		{name: "lint", summary: "static checks", run: "make lint"},
		{name: "fmt", summary: "static checks", run: "make fmt"},
		{name: "check", summary: "fmt + vet", run: "make check"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("make targets\nwant %+v\ngot  %+v", want, got)
	}
}

func TestJustRecipes(t *testing.T) {
	justfile := `set shell := ["bash", "-c"]
alias b := build
version := "1.0"
export TOKEN := "x"

# Builds the binary
build target='debug':
    go build

[doc('Deploys to an environment')]
[group('ops')]
deploy env +flags:
    ./deploy.sh {{env}}

[private]
helper:
    true

_hidden:
    true

@quiet:
    echo hi

url := 'http://a:b'
`
	got := justRecipes(justfile)
	want := []runner{
		{name: "build", summary: "Builds the binary", run: "just build"},
		{name: "deploy", summary: "Deploys to an environment", run: "just deploy"},
		{name: "quiet", summary: "", run: "just quiet"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("just recipes\nwant %+v\ngot  %+v", want, got)
	}
}

func TestNpmScripts(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := []byte(`{"scripts": {"test": "vitest  run", "postinstall": "patch-package", "build:prod": "vite build --mode production"}}`)
	got := npmScripts(manifest, packageManager(app, root))
	want := []runner{
		{name: "build:prod", summary: "vite build --mode production", run: "pnpm run build:prod"},
		{name: "test", summary: "vitest run", run: "pnpm run test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("npm scripts\nwant %+v\ngot  %+v", want, got)
	}

	if manager := packageManager(t.TempDir(), root); manager != "npm" {
		t.Fatalf("no lockfile: want npm, got %s", manager)
	}
	if npmScripts([]byte("{not json"), "npm") != nil {
		t.Fatal("broken package.json must give no scripts")
	}
}

func TestTaskfileTasks(t *testing.T) {
	taskfile := []byte(`version: '3'
tasks:
  build:
    desc: Build the binary
    cmds: [go build]
  release:
    summary: |
      Tag a new version.
      Then publish it.
  setup:
    internal: true
  "db:up": docker compose up -d
`)
	got := taskfileTasks(taskfile)
	want := []runner{
		{name: "build", summary: "Build the binary", run: "task build"},
		{name: "release", summary: "Tag a new version", run: "task release"},
		{name: "db:up", summary: "", run: "task db:up"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tasks\nwant %+v\ngot  %+v", want, got)
	}
}
