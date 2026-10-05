package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunPassesArgumentsUntouched(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"space":       {"-q", "a b"},
		"dollar":      {"-q", "$HOME"},
		"glob":        {"*"},
		"empty":       {"-q", ""},
		"double dash": {"--yes"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			s.wantArgs(args, append([]string{"run", "probe-ro", "--"}, args...)...)
		})
	}
}

func TestRunSemicolonDoesNotStartASecondCommand(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	payload := "x;touch " + s.path("pwned")
	s.wantArgs([]string{payload}, "run", "probe-ro", "--", payload)
	if s.exists("pwned") {
		t.Fatal("the second command from the argument ran")
	}
}

func TestRunArgumentsWithoutDoubleDashReachTheTool(t *testing.T) {
	t.Parallel()
	newSandbox(t).wantArgs([]string{"-x", "1"}, "run", "probe-ro", "-x", "1")
}

func TestRunStartsFromWorkdirWithRunIDAndTool(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	probe := s.jig("run", "probe-ro").ran(t)

	want, _ := filepath.EvalSymlinks(s.path("probes/probe-ro"))
	got, _ := filepath.EvalSymlinks(probe.Cwd)
	if got != want {
		t.Fatalf("cwd: want %s, got %s", want, got)
	}
	if probe.RunID == "" {
		t.Fatal("JIG_RUN_ID was not passed to the tool")
	}
	if probe.Tool != "probe-ro" {
		t.Fatalf("JIG_TOOL: want probe-ro, got %q", probe.Tool)
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	r := s.jigWith(s.root, []string{"JIG_PROBE_EXIT=3"}, "", "run", "probe-ro")
	r.ran(t)
	if r.code != 3 {
		t.Fatalf("exit code: want 3, got %d", r.code)
	}
}

func TestRunNeedsExactID(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.wantBlocked("no tool", "run", "nope")
	s.wantBlocked("probe-writes", "run", "--yes", "probe-wri")
}

func TestShowFindsPartialID(t *testing.T) {
	t.Parallel()
	r := newSandbox(t).jig("show", "probe-wri")
	r.ok(t)
	contains(t, "stdout", r.stdout, "probe-writes")
}

func TestConfirmation(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, guardedFixture)

	s.wantBlocked("--yes", "run", "probe-writes")
	s.wantArgs([]string{}, "run", "--yes", "probe-writes")
	s.wantArgs([]string{}, "run", "probe-writes", "--yes")
	s.wantBlocked("--yes", "run", "probe-writes", "--", "--yes")
	s.wantArgs([]string{"--yes"}, "run", "--yes", "probe-writes", "--", "--yes")
	s.wantArgs([]string{}, "run", "probe-ro")
	s.wantArgs([]string{}, "run", "probe-dev")
	s.wantArgs([]string{"-env", "prd"}, "run", "probe-guarded", "--", "-env", "prd")
}

func TestProdTargetsComeFromConfig(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.write("jig.yml", "prod_targets: [dev]\n")

	s.wantArgs([]string{}, "run", "probe-writes")
	s.wantBlocked("--yes", "run", "probe-dev")
}

func TestShowJSON(t *testing.T) {
	t.Parallel()
	r := newSandbox(t, guardedFixture).jig("show", "--json", "probe-guarded")
	r.ok(t)
	var tool map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &tool); err != nil {
		t.Fatal(err)
	}
	if tool["guard"] != "self" || tool["needs_confirm"] != false {
		t.Fatalf("guard=%v needs_confirm=%v", tool["guard"], tool["needs_confirm"])
	}
}

func TestUnknownGuardIsABrokenManifest(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.write(".jig/registry/bad-guard.yml", "id: bad-guard\npath: probes/probe-ro\nsummary: s\nrun: 'true'\nguard: maybe\n")
	item := findKind(s.doctor(), "broken manifest")
	if item == nil {
		t.Fatal("doctor missed guard: maybe")
	}
	contains(t, "detail", item.Detail, "guard")
}

func TestList(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)

	r := s.jig("ls")
	r.ok(t)
	contains(t, "ls", r.stdout, "probe-ro")
	lacks(t, "ls", r.stdout, "probe-deprecated")
	lacks(t, "ls", r.stdout, "probe-env")

	r = s.jig("ls", "--all")
	contains(t, "ls --all", r.stdout, "probe-deprecated")
	contains(t, "ls --all", r.stdout, "probe-env")

	r = s.jig("ls", "--kind", "env")
	contains(t, "ls --kind env", r.stdout, "probe-env")
	lacks(t, "ls --kind env", r.stdout, "probe-ro")

	r = s.jig("ls", "probe", "dev")
	contains(t, "ls probe dev", r.stdout, "probe-dev")
	lacks(t, "ls probe dev", r.stdout, "probe-ro")
}

func TestListJSONHasStableFields(t *testing.T) {
	t.Parallel()
	r := newSandbox(t).jig("ls", "--json")
	r.ok(t)
	var tools []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &tools); err != nil || len(tools) == 0 {
		t.Fatalf("ls --json: %v, %d tools", err, len(tools))
	}
	for _, tool := range tools {
		for _, key := range []string{"id", "summary", "run", "safety", "manifest_path", "workdir_abs", "needs_confirm"} {
			if _, found := tool[key]; !found {
				t.Fatalf("%v has no %q", tool["id"], key)
			}
		}
	}

	r = newSandbox(t).jig("ls", "--json", "--search", "nothing-like-this")
	if strings.TrimSpace(r.stdout) != "[]" {
		t.Fatalf("an empty search must print [], got %q", r.stdout)
	}
}

func TestHomeWithoutTTYPrintsTheList(t *testing.T) {
	t.Parallel()
	r := newSandbox(t).jig()
	r.ok(t)
	contains(t, "stdout", r.stdout, "probe-ro")
}

func TestNoWorkspace(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	dir := t.TempDir()
	env := []string{"JIG_ROOT="}
	r := s.jigWith(dir, env, "", "ls")
	r.failed(t)
	contains(t, "stderr", r.stderr, "jig init")
}

func TestWorkspaceIsFoundFromASubdirectory(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	r := s.jigWith(s.path("probes/probe-ro"), []string{"JIG_ROOT="}, "", "ls")
	r.ok(t)
	contains(t, "stdout", r.stdout, "probe-ro")
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("help creates nothing", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		r := s.jig("new", "--help")
		r.ok(t)
		contains(t, "stdout", r.stdout, "jig new")
		if s.exists(".jig/registry/--help.yml") || s.exists("scripts/--help") {
			t.Fatal("jig new --help created a tool")
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jig("new", "Bad Id").failed(t)
		if s.exists(".jig/registry/Bad Id.yml") {
			t.Fatal("manifest created for an invalid id")
		}
	})

	t.Run("scaffold with guard self", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jig("new", "fresh-tool", "--dir", "tools/fresh-tool").ok(t)
		for _, rel := range []string{"tools/fresh-tool/main.go", "tools/fresh-tool/screen.go", ".jig/registry/fresh-tool.yml"} {
			if !s.exists(rel) {
				t.Fatalf("not created: %s", rel)
			}
		}
		manifest, _ := os.ReadFile(s.path(".jig/registry/fresh-tool.yml"))
		contains(t, "manifest", string(manifest), "guard: self")
	})

	t.Run("outside the workspace", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		outside := filepath.Join(filepath.Dir(s.root), "outside")
		s.jig("new", "outside-tool", "--dir", outside).failed(t)
		if _, err := os.Stat(outside); err == nil {
			t.Fatal("a directory was left outside the workspace")
		}
	})

	t.Run("a word from another why is not similar", func(t *testing.T) {
		t.Parallel()
		newSandbox(t).jig("new", "widgets-sync", "--dir", "tools/widgets-sync").ok(t)
	})

	t.Run("a generic word is not similar", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, fixture{id: "sh-tool"})
		s.jig("new", "js-tool", "--dir", "tools/js-tool").ok(t)
	})

	t.Run("a shared id word stops creation", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		r := s.jig("new", "probe-extra", "--dir", "tools/probe-extra")
		r.failed(t)
		contains(t, "stderr", r.stderr, "similar")
		if s.exists("tools/probe-extra") || s.exists(".jig/registry/probe-extra.yml") {
			t.Fatal("files were left behind")
		}
	})

	t.Run("a tag counts as similar", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, fixture{id: "stock-diag", tags: "[inventory]"})
		s.jig("new", "inventory-sync", "--dir", "tools/inventory-sync").failed(t)
	})

	t.Run("envs from the config", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.write("jig.yml", "envs: [qa, live]\n")
		s.jig("new", "env-aware", "--kind", "bash", "--dir", "tools/env-aware").ok(t)
		body, _ := os.ReadFile(s.path("tools/env-aware/main.sh"))
		contains(t, "main.sh", string(body), "qa|live")
		manifest, _ := os.ReadFile(s.path(".jig/registry/env-aware.yml"))
		contains(t, "manifest", string(manifest), "targets: [qa]")
	})
}

func TestDoctor(t *testing.T) {
	t.Parallel()

	t.Run("clean right after index", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jig("index").ok(t)
		if problems := s.problems(); len(problems) > 0 {
			t.Fatalf("expected a clean doctor, got %v", problems)
		}
	})

	t.Run("broken manifest", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.write(".jig/registry/broken.yml", "id: [\n")
		if findKind(s.doctor(), "broken manifest") == nil {
			t.Fatal("not found")
		}
	})

	t.Run("script without manifest", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.write("svc/scripts/orphan.sh", "#!/bin/sh\necho orphan\n")
		item := findKind(s.doctor(), "script without manifest")
		if item == nil || len(item.Paths) != 1 || item.Paths[0] != "svc/scripts/orphan.sh" {
			t.Fatalf("got %+v", item)
		}
	})

	t.Run("jigignore and skip silence orphans", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.write("svc/scripts/orphan.sh", "#!/bin/sh\n")
		s.write("legacy/scripts/old.sh", "#!/bin/sh\n")
		s.write(".jigignore", "svc/scripts/orphan.sh\n")
		s.write("jig.yml", "skip: [/legacy]\n")
		if item := findKind(s.doctor(), "script without manifest"); item != nil {
			t.Fatalf("still reported: %v", item.Paths)
		}
	})

	t.Run("scaffolded screen.go is not a duplicate", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jig("new", "alpha-one", "--dir", "tools/alpha-one").ok(t)
		s.jig("new", "beta-two", "--dir", "tools/beta-two").ok(t)
		if item := findKind(s.doctor(), "identical files"); item != nil {
			t.Fatalf("reported %v", item.Paths)
		}
	})

	t.Run("stale index", func(t *testing.T) {
		t.Parallel()
		if findKind(newSandbox(t).doctor(), "TOOLS.md is stale") == nil {
			t.Fatal("not found")
		}
	})
}

func TestIndexPutsDeprecatedLast(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.jig("index").ok(t)
	raw, err := os.ReadFile(s.path("TOOLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	contains(t, "TOOLS.md", text, "`probe-ro`")
	contains(t, "TOOLS.md", text, "## Deprecated")
	if strings.Index(text, "`probe-deprecated`") < strings.Index(text, "## Deprecated") {
		t.Fatal("probe-deprecated is above the Deprecated section")
	}
}

func TestRunLog(t *testing.T) {
	t.Parallel()

	t.Run("exit code of the tool", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jigWith(s.root, []string{"JIG_PROBE_EXIT=3"}, "", "run", "probe-ro")
		records := s.runlog()
		if len(records) == 0 {
			t.Fatal("run log is empty")
		}
		last := records[len(records)-1]
		for key, want := range map[string]any{"source": "jig", "cmd": "run", "tool": "probe-ro", "exit_code": float64(3)} {
			if last[key] != want {
				t.Fatalf("%s=%v, want %v", key, last[key], want)
			}
		}
	})

	t.Run("secret flags are masked", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.wantArgs([]string{"-token", "s3cr3t", "-x", "1"}, "run", "probe-ro", "--", "-token", "s3cr3t", "-x", "1")
		raw, _ := os.ReadFile(s.logPath)
		lacks(t, "run log", string(raw), "s3cr3t")
		contains(t, "run log", string(raw), "***")
	})

	t.Run("JIG_LOG=- disables it", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		s.jigWith(s.root, []string{"JIG_LOG=-"}, "", "run", "probe-ro")
		if _, err := os.Stat(s.logPath); err == nil {
			t.Fatal("run log written although JIG_LOG=-")
		}
	})
}

func TestInit(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	dir := t.TempDir()
	r := s.jigWith(dir, []string{"JIG_ROOT="}, "", "init")
	r.ok(t)
	for _, rel := range []string{"jig.yml", ".jig/registry"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("not created: %s", rel)
		}
	}
	s.jigWith(dir, []string{"JIG_ROOT="}, "", "init").failed(t)
}

func TestAgentTexts(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	contains(t, "rules", s.jig("agent", "rules").stdout, "jig ls")
	contains(t, "skill", s.jig("agent", "skill").stdout, "name: jig")
	s.jig("agent", "nope").failed(t)
}

func TestHook(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.write("svc/scripts/existing.sh", "#!/bin/sh\n")

	hook := func(tool, file string) string {
		input, _ := json.Marshal(map[string]any{"tool_name": tool, "cwd": s.root, "tool_input": map[string]string{"file_path": file}})
		r := s.jigWith(s.root, nil, string(input), "hook")
		r.ok(t)
		return r.stdout
	}

	out := hook("Write", s.path("svc/scripts/stock-recount/main.go"))
	contains(t, "new script", out, `"permissionDecision":"deny"`)
	contains(t, "new script", out, "jig ls stock recount")

	for name, file := range map[string]string{
		"existing file":       s.path("svc/scripts/existing.sh"),
		"not a tool dir":      s.path("svc/internal/handler.go"),
		"inside a known tool": s.path("probes/probe-ro/helper.sh"),
		"outside workspace":   filepath.Join(t.TempDir(), "x.sh"),
		"vendor":              s.path("svc/vendor/scripts/x.sh"),
	} {
		if out := hook("Write", file); out != "" {
			t.Fatalf("%s: expected no decision, got %s", name, out)
		}
	}
	if out := hook("Edit", s.path("svc/scripts/new.sh")); out != "" {
		t.Fatalf("Edit must pass, got %s", out)
	}
	r := s.jigWith(s.root, []string{"JIG_HOOK=off"}, `{"tool_name":"Write","tool_input":{"file_path":"`+s.path("svc/scripts/n.sh")+`"}}`, "hook")
	if r.stdout != "" {
		t.Fatalf("JIG_HOOK=off must pass, got %s", r.stdout)
	}
}

func TestDemoPlain(t *testing.T) {
	t.Parallel()
	r := newSandbox(t).jig("demo", "--only", "fan", "--speed", "20")
	r.ok(t)
	contains(t, "stderr", r.stderr, "shard-6: connection refused")
	lacks(t, "stderr", r.stderr, "\x1b[")
}

func TestVersionWorksAnywhere(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	for _, command := range []string{"version", "--version"} {
		r := s.jigWith(t.TempDir(), []string{"JIG_ROOT="}, "", command)
		r.ok(t)
		if !strings.HasPrefix(r.stdout, "jig ") {
			t.Fatalf("%s: %q", command, r.stdout)
		}
		contains(t, command, r.stdout, runtime.GOOS+"/"+runtime.GOARCH)
	}
}

func TestDoctorExitCode(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	if r := s.jig("doctor"); r.code != 1 {
		t.Fatalf("a stale index is a problem, doctor exited %d", r.code)
	}
	s.jig("index").ok(t)
	s.jig("doctor").ok(t)
}

func TestIndexIsComparedByContent(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.jig("index").ok(t)
	first, _ := os.ReadFile(s.path("TOOLS.md"))
	lacks(t, "TOOLS.md", string(first), "updated")

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(s.path(".jig/registry/probe-ro.yml"), later, later); err != nil {
		t.Fatal(err)
	}
	if item := findKind(s.doctor(), "TOOLS.md is stale"); item != nil {
		t.Fatal("a newer mtime with the same content is not stale")
	}

	s.jig("index").ok(t)
	second, _ := os.ReadFile(s.path("TOOLS.md"))
	if string(first) != string(second) {
		t.Fatal("jig index is not deterministic")
	}

	manifest, _ := os.ReadFile(s.path(".jig/registry/probe-ro.yml"))
	s.write(".jig/registry/probe-ro.yml", strings.Replace(string(manifest), "summary: fixture probe-ro", "summary: counts widgets", 1))
	if findKind(s.doctor(), "TOOLS.md is stale") == nil {
		t.Fatal("a changed summary must make the index stale")
	}
}

func TestFreshWorkspaceIsClean(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	dir := t.TempDir()
	s.jigWith(dir, []string{"JIG_ROOT="}, "", "init").ok(t)
	r := s.jigWith(dir, []string{"JIG_ROOT="}, "", "doctor")
	r.ok(t)
	contains(t, "doctor", r.stdout, "clean")
}

func TestNewManifestIsUnfinished(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.jig("new", "fresh-tool", "--kind", "bash", "--dir", "tools/fresh-tool").ok(t)
	item := findKind(s.doctor(), "unfinished manifest")
	if item == nil || len(item.Paths) != 1 || item.Paths[0] != ".jig/registry/fresh-tool.yml: summary, why" {
		t.Fatalf("got %+v", item)
	}
}

func TestGoScaffoldsBuild(t *testing.T) {
	t.Parallel()
	for _, envs := range []string{"", "envs: [qa, live]\n"} {
		for _, kind := range []string{"go", "go-parallel"} {
			t.Run(kind+" "+strings.TrimSpace(envs), func(t *testing.T) {
				t.Parallel()
				s := newSandbox(t)
				s.write("jig.yml", envs)
				s.jig("new", "fresh-"+kind, "--kind", kind, "--dir", "tools/fresh").ok(t)

				if !s.exists("tools/fresh/go.mod") {
					t.Fatal("a scaffold outside any Go module needs its own go.mod")
				}
				tool := s.show("fresh-" + kind)
				if tool["run"] != "go run ." || tool["workdir_abs"] != s.path("tools/fresh") {
					t.Fatalf("run=%v from %v", tool["run"], tool["workdir_abs"])
				}
				goBuild(t, s.path("tools/fresh"), ".")
			})
		}
	}
}

func TestGoScaffoldJoinsTheModuleAboveIt(t *testing.T) {
	t.Parallel()
	s := newRepoSandbox(t)
	s.write("go.mod", "module example.com/workspace\n\ngo 1.24\n")
	s.jig("new", "inner-tool", "--dir", "svc/scripts/inner-tool").ok(t)

	if s.exists("svc/scripts/inner-tool/go.mod") {
		t.Fatal("go.mod written inside an existing module")
	}
	if run := s.show("inner-tool")["run"]; run != "go run ./svc/scripts/inner-tool" {
		t.Fatalf("run=%v", run)
	}
	goBuild(t, s.root, "./svc/scripts/inner-tool")
}

func TestShellScaffoldsParse(t *testing.T) {
	t.Parallel()
	s := newSandbox(t)
	s.write("jig.yml", "envs: [qa, live]\n")
	s.jig("new", "shell-tool", "--kind", "bash", "--dir", "tools/shell-tool").ok(t)
	s.jig("new", "node-tool", "--kind", "node", "--dir", "tools/node-tool").ok(t)

	check := func(name string, args ...string) {
		if _, err := exec.LookPath(name); err != nil {
			t.Logf("%s is not installed, skipping", name)
			return
		}
		if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	check("bash", "-n", s.path("tools/shell-tool/main.sh"))
	check("node", "--check", s.path("tools/node-tool/run.js"))
}

func goBuild(t *testing.T, dir, pkg string) {
	t.Helper()
	build := exec.Command("go", "build", "-o", os.DevNull, pkg)
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod -buildvcs=false")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build %s in %s: %v\n%s", pkg, dir, err, out)
	}
}
