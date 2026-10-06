package e2e

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func newFoundSandbox(t *testing.T) *sandbox {
	t.Helper()
	s := newRepoSandbox(t)
	s.write("ops/scripts/check-stock.sh", "#!/bin/sh\n# check-stock.sh — tells why a published item shows out of stock.\n# Reads every shard.\necho stock-checked \"$@\"\n")
	s.write("Makefile", ".PHONY: hello\nhello: ## say hello from make\n\t@echo hello-from-make\n")
	s.write("web/package.json", `{"scripts": {"lint": "eslint .", "postinstall": "patch-package"}}`)
	s.write("tools/helpers.py", "def helper():\n    return 1\n")
	s.write("tools/ui/app.js", "document.title = 'x'\n")
	s.write("tools/report/main.go", "// Command report sums last week's refunds.\npackage main\n\nfunc main() {}\n")
	s.write("tools/report/util.go", "package main\n")
	return s
}

func (s *sandbox) listJSON(args ...string) map[string]map[string]any {
	s.t.Helper()
	r := s.jig(append([]string{"ls", "--json"}, args...)...)
	r.ok(s.t)
	var tools []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &tools); err != nil {
		s.t.Fatalf("ls --json: %v\n%s", err, r.stdout)
	}
	byID := map[string]map[string]any{}
	for _, tool := range tools {
		byID[tool["id"].(string)] = tool
	}
	return byID
}

func TestListFindsWhatHasNoManifest(t *testing.T) {
	t.Parallel()
	s := newFoundSandbox(t)

	r := s.jig("ls", "stock")
	r.ok(t)
	contains(t, "ls stock", r.stdout, "found in the workspace, not registered")
	contains(t, "ls stock", r.stdout, "ops/scripts/check-stock.sh")
	contains(t, "ls stock", r.stdout, "tells why a published item shows out of stock")

	found := s.listJSON()
	cases := map[string]struct{ origin, run, summary string }{
		"ops/scripts/check-stock.sh": {"script", "bash ./ops/scripts/check-stock.sh", "tells why a published item shows out of stock"},
		"Makefile:hello":             {"make", "make hello", "say hello from make"},
		"web/package.json:lint":      {"npm", "npm run lint", "eslint ."},
		"tools/report":               {"script", "go run ./tools/report", "report sums last week's refunds"},
	}
	for id, want := range cases {
		tool, ok := found[id]
		if !ok {
			t.Fatalf("%s is not listed", id)
		}
		if tool["registered"] != false || tool["origin"] != want.origin || tool["run"] != want.run || tool["summary"] != want.summary {
			t.Errorf("%s: registered=%v origin=%v run=%v summary=%v", id, tool["registered"], tool["origin"], tool["run"], tool["summary"])
		}
	}
	if found["probe-ro"]["registered"] != true || found["probe-ro"]["origin"] != "registry" {
		t.Errorf("probe-ro: registered=%v origin=%v", found["probe-ro"]["registered"], found["probe-ro"]["origin"])
	}
	for _, id := range []string{"tools/helpers.py", "tools/ui/app.js", "tools/report/util.go", "web/package.json:postinstall"} {
		if _, listed := found[id]; listed {
			t.Errorf("%s is not something to run", id)
		}
	}

	if _, listed := s.listJSON("--registered")["Makefile:hello"]; listed {
		t.Error("--registered lists a make target")
	}
	onlyFound := s.listJSON("--found")
	if _, listed := onlyFound["probe-ro"]; listed {
		t.Error("--found lists a registered tool")
	}
	if _, listed := s.listJSON("--tag", "anything")["Makefile:hello"]; listed {
		t.Error("a tag filter cannot match a found entry")
	}
}

func TestFoundSkipsRegisteredAndIgnored(t *testing.T) {
	t.Parallel()
	s := newFoundSandbox(t)
	s.write(".jigignore", "Makefile\n")
	s.write("jig.yml", "skip: [web]\n")
	s.jig("add", "ops/scripts/check-stock.sh", "--id", "check-stock").ok(t)

	found := s.listJSON()
	for _, id := range []string{"ops/scripts/check-stock.sh", "Makefile:hello", "web/package.json:lint"} {
		if _, listed := found[id]; listed {
			t.Errorf("%s is still listed as found", id)
		}
	}
	if found["check-stock"]["registered"] != true {
		t.Fatal("the registered script is missing")
	}
}

func TestShowAndRunFoundEntries(t *testing.T) {
	t.Parallel()
	s := newFoundSandbox(t)

	r := s.jig("show", "check-stock")
	r.ok(t)
	contains(t, "show", r.stdout, "ops/scripts/check-stock.sh")
	contains(t, "show", r.stdout, "jig add ops/scripts/check-stock.sh")

	r = s.jig("run", "ops/scripts/check-stock.sh", "--", "a b")
	r.ok(t)
	contains(t, "run", r.stdout, "stock-checked a b")
	contains(t, "run", r.stderr, "not registered")

	s.wantBlocked("exact id", "run", "check-stock")

	r = s.jigWith(s.path("ops"), nil, "", "run", "scripts/check-stock.sh")
	r.ok(t)
	contains(t, "run from a subdirectory", r.stdout, "stock-checked")

	if _, err := exec.LookPath("make"); err != nil {
		t.Log("make is not installed, skipping the make target")
		return
	}
	r = s.jig("run", "Makefile:hello")
	r.ok(t)
	contains(t, "make", r.stdout, "hello-from-make")
}

func TestInitReportsWhatItFound(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	dir := t.TempDir()
	workspace := &sandbox{t: t, root: dir}
	workspace.write("scripts/backup.sh", "#!/bin/sh\n# copies the nightly dump to the replica\n")
	workspace.write("scripts/plain.sh", "#!/bin/sh\necho\n")
	workspace.write("Makefile", "build:\n\tgo build\n")

	r := s.jigWith(dir, []string{"JIG_ROOT="}, "", "init")
	r.ok(t)
	contains(t, "init", r.stdout, "found 2 scripts (1 with a description), 1 make target")

	r = s.jigWith(dir, []string{"JIG_ROOT="}, "", "ls", "nightly")
	r.ok(t)
	contains(t, "ls after init", r.stdout, "scripts/backup.sh")
}

func TestAddTakesTheSummaryFromTheHeader(t *testing.T) {
	t.Parallel()
	s := newFoundSandbox(t)
	s.jig("add", "ops/scripts/check-stock.sh").ok(t)

	if summary := s.show("check-stock")["summary"]; summary != "tells why a published item shows out of stock" {
		t.Fatalf("summary=%v", summary)
	}
	item := findKind(s.doctor(), "unfinished manifest")
	if item == nil || len(item.Paths) != 1 || item.Paths[0] != ".jig/registry/check-stock.yml: why" {
		t.Fatalf("got %+v", item)
	}
}
