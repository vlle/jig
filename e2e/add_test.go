package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newRepoSandbox(t *testing.T) *sandbox {
	t.Helper()
	s := newSandbox(t)
	s.write(".git/HEAD", "ref: refs/heads/main\n")
	return s
}

func (s *sandbox) show(id string) map[string]any {
	s.t.Helper()
	r := s.jig("show", "--json", id)
	r.ok(s.t)
	var tool map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &tool); err != nil {
		s.t.Fatal(err)
	}
	return tool
}

func (s *sandbox) executable(rel string) {
	s.t.Helper()
	if err := os.Chmod(s.path(rel), 0o755); err != nil {
		s.t.Fatal(err)
	}
}

func TestAddGuessesIDAndRun(t *testing.T) {
	t.Parallel()
	s := newRepoSandbox(t)
	s.write("tools/hello.sh", "#!/bin/sh\necho hello \"$@\"\n")
	s.executable("tools/hello.sh")
	s.write("tools/count.py", "print(1)\n")
	s.write("tools/plain.sh", "echo plain\n")
	s.write("tools/fetch/run.js", "console.log(1)\n")
	s.write("svc/scripts/report/main.go", "package main\n\nfunc main() {}\n")
	s.write("deep/mod/go.mod", "module deep\n\ngo 1.24\n")
	s.write("deep/mod/cmd/x/main.go", "package main\n\nfunc main() {}\n")

	s.jig("add", "tools/hello.sh", "tools/count.py", "tools/plain.sh", "tools/fetch/run.js", "svc/scripts/report", "deep/mod/cmd/x").ok(t)

	cases := map[string]struct{ run, workdir string }{
		"hello":  {"./tools/hello.sh", "."},
		"count":  {"python3 ./tools/count.py", "."},
		"plain":  {"bash ./tools/plain.sh", "."},
		"fetch":  {"node ./tools/fetch/run.js", "."},
		"report": {"go run ./svc/scripts/report", "."},
		"x":      {"go run ./cmd/x", "deep/mod"},
	}
	for id, want := range cases {
		tool := s.show(id)
		if tool["run"] != want.run {
			t.Errorf("%s: run=%v, want %s", id, tool["run"], want.run)
		}
		if got, _ := filepath.Rel(s.root, tool["workdir_abs"].(string)); got != want.workdir {
			t.Errorf("%s: workdir=%s, want %s", id, got, want.workdir)
		}
	}

	r := s.jig("run", "hello", "--", "a b")
	r.ok(t)
	contains(t, "stdout", r.stdout, "hello a b")
}

func TestAddWithDescriptionIsClean(t *testing.T) {
	t.Parallel()
	s := newRepoSandbox(t)
	s.write("ops/scripts/rotate.sh", "#!/bin/sh\n")

	r := s.jig("add", "ops/scripts/rotate.sh", "--id", "key-rotate", "--summary", "rotate: the signing key", "--why", "keys expire\nevery 90 days",
		"--safety", "destructive", "--targets", "stage,prd", "--tags", "keys, ops")
	r.ok(t)

	tool := s.show("key-rotate")
	for key, want := range map[string]any{"summary": "rotate: the signing key", "why": "keys expire\nevery 90 days\n", "safety": "destructive", "needs_confirm": true} {
		if tool[key] != want {
			t.Errorf("%s=%q, want %q", key, tool[key], want)
		}
	}
	tags, _ := json.Marshal(tool["tags"])
	if string(tags) != `["keys","ops"]` {
		t.Errorf("tags=%s", tags)
	}

	s.jig("index").ok(t)
	if problems := s.problems(); len(problems) > 0 {
		t.Fatalf("expected a clean doctor, got %+v", problems)
	}
}

func TestAddLeavesTODOsForDoctor(t *testing.T) {
	t.Parallel()
	s := newRepoSandbox(t)
	s.write("tools/sync.sh", "#!/bin/sh\n")
	s.jig("add", "tools/sync.sh").ok(t)

	if s.show("sync")["safety"] != "writes" {
		t.Fatal("an existing script must not be assumed read-only")
	}
	item := findKind(s.doctor(), "unfinished manifest")
	if item == nil || len(item.Paths) != 1 || item.Paths[0] != ".jig/registry/sync.yml: summary, why" {
		t.Fatalf("got %+v", item)
	}
}

func TestAddRefuses(t *testing.T) {
	t.Parallel()
	s := newRepoSandbox(t)
	s.write("tools/good.sh", "#!/bin/sh\n")
	s.write("other/probe-ro.sh", "#!/bin/sh\n")
	s.write("tools/README", "not a script\n")

	cases := map[string][]string{
		"already registered":   {"probes/probe-ro/main.sh"},
		"id is taken":          {"other/probe-ro.sh"},
		"missing path":         {"tools/nope.sh"},
		"outside":              {filepath.Join(t.TempDir())},
		"workspace root":       {"."},
		"unknown safety":       {"tools/good.sh", "--safety", "maybe"},
		"one id for two paths": {"tools/good.sh", "other/probe-ro.sh", "--id", "both"},
		"cannot guess run":     {"tools/README"},
		"unknown flag":         {"tools/good.sh", "--force"},
		"one bad path of two":  {"tools/good.sh", "tools/nope.sh"},
	}
	for name, args := range cases {
		r := s.jig(append([]string{"add"}, args...)...)
		if r.code == 0 {
			t.Errorf("%s: jig add %v succeeded", name, args)
		}
	}
	if s.exists(".jig/registry/good.yml") || s.exists(".jig/registry/both.yml") {
		t.Fatal("a manifest was written although jig add failed")
	}
}
