package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	jigBin  string
	selfBin string
)

type probeReport struct {
	Args  []string `json:"args"`
	Cwd   string   `json:"cwd"`
	RunID string   `json:"run_id"`
	Tool  string   `json:"tool"`
}

func TestMain(m *testing.M) {
	if os.Getenv("JIG_E2E_PROBE") == "1" {
		os.Exit(runProbe(os.Args[1:]))
	}
	os.Exit(setupAndRun(m))
}

func setupAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "jig-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	jigBin = filepath.Join(dir, "jig")
	build := exec.Command("go", "build", "-o", jigBin, ".")
	build.Dir = ".."
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build jig: %v\n%s", buildErr, out)
		return 1
	}

	if selfBin, err = os.Executable(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func runProbe(args []string) int {
	cwd, _ := os.Getwd()
	report := probeReport{Args: args, Cwd: cwd, RunID: os.Getenv("JIG_RUN_ID"), Tool: os.Getenv("JIG_TOOL")}
	if report.Args == nil {
		report.Args = []string{}
	}
	raw, _ := json.Marshal(report)
	if err := os.WriteFile(os.Getenv("JIG_PROBE_OUT"), raw, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	code, _ := strconv.Atoi(os.Getenv("JIG_PROBE_EXIT"))
	return code
}

type fixture struct {
	id, kind, safety, targets, guard, status, why, tags string
}

var baseFixtures = []fixture{
	{id: "probe-ro", safety: "read-only", targets: "[prd]", why: "reads prod, counts widgets"},
	{id: "probe-writes", safety: "writes", targets: "[stage, prd]"},
	{id: "probe-dev", safety: "destructive", targets: "[dev]"},
	{id: "probe-deprecated", targets: "[local]", status: "deprecated"},
	{id: "probe-env", kind: "env", targets: "[stage]"},
}

var guardedFixture = fixture{id: "probe-guarded", safety: "writes", targets: "[stage, prd]", guard: "self"}

type sandbox struct {
	t        *testing.T
	root     string
	logPath  string
	probeOut string
}

type result struct {
	code   int
	stdout string
	stderr string
	probe  *probeReport
}

func newSandbox(t *testing.T, extra ...fixture) *sandbox {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &sandbox{
		t:        t,
		root:     filepath.Join(dir, "root"),
		logPath:  filepath.Join(dir, "runs.jsonl"),
		probeOut: filepath.Join(dir, "probe.json"),
	}
	s.write("jig.yml", "")
	for _, item := range append(slices.Clone(baseFixtures), extra...) {
		s.addFixture(item)
	}
	return s
}

func (s *sandbox) path(rel string) string {
	return filepath.Join(s.root, rel)
}

func (s *sandbox) write(rel, content string) {
	s.t.Helper()
	target := s.path(rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sandbox) exists(rel string) bool {
	_, err := os.Stat(s.path(rel))
	return err == nil
}

func (s *sandbox) addFixture(item fixture) {
	s.write(filepath.Join("probes", item.id, "main.sh"), "#!/bin/sh\necho "+item.id+"\n")

	var manifest strings.Builder
	fmt.Fprintf(&manifest, "id: %s\npath: probes/%s\nsummary: fixture %s\n", item.id, item.id, item.id)
	raw, _ := json.Marshal("'" + strings.ReplaceAll(selfBin, "'", `'\''`) + "'")
	fmt.Fprintf(&manifest, "run: %s\n", raw)
	for _, field := range [][2]string{
		{"kind", item.kind}, {"safety", item.safety}, {"targets", item.targets},
		{"guard", item.guard}, {"status", item.status}, {"why", item.why}, {"tags", item.tags},
	} {
		if field[1] != "" {
			fmt.Fprintf(&manifest, "%s: %s\n", field[0], field[1])
		}
	}
	s.write(filepath.Join(".jig", "registry", item.id+".yml"), manifest.String())
}

func (s *sandbox) jig(args ...string) result {
	return s.jigWith(s.root, nil, "", args...)
}

func (s *sandbox) jigWith(cwd string, env []string, stdin string, args ...string) result {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_ = os.Remove(s.probeOut)

	cmd := exec.CommandContext(ctx, jigBin, args...)
	cmd.Dir = cwd
	cmd.Env = append(s.environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	res := result{}
	runErr := cmd.Run()
	res.stdout, res.stderr = stdout.String(), stderr.String()

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr) && ctx.Err() == nil:
		res.code = exitErr.ExitCode()
	default:
		s.t.Fatalf("jig %s: %v", strings.Join(args, " "), runErr)
	}

	raw, err := os.ReadFile(s.probeOut)
	if err == nil {
		var report probeReport
		if err := json.Unmarshal(raw, &report); err != nil {
			s.t.Fatalf("probe report: %v", err)
		}
		res.probe = &report
	}
	return res
}

func (s *sandbox) environ() []string {
	env := make([]string, 0, len(os.Environ())+8)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "JIG_") || name == "NO_COLOR" || name == "TERM" || name == "XDG_STATE_HOME" {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"JIG_ROOT="+s.root,
		"JIG_LOG="+s.logPath,
		"JIG_PROBE_OUT="+s.probeOut,
		"JIG_E2E_PROBE=1",
		"XDG_STATE_HOME="+filepath.Join(filepath.Dir(s.root), "state"),
		"NO_COLOR=1",
		"TERM=dumb",
	)
}

func (s *sandbox) runlog() []map[string]any {
	s.t.Helper()
	file, err := os.Open(s.logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			s.t.Fatalf("run log line is not JSON: %v", err)
		}
		records = append(records, record)
	}
	return records
}

type doctorFinding struct {
	Kind   string   `json:"kind"`
	Level  string   `json:"level"`
	Detail string   `json:"detail"`
	Paths  []string `json:"paths"`
}

func (s *sandbox) doctor() []doctorFinding {
	s.t.Helper()
	r := s.jig("doctor", "--json")
	r.ok(s.t)
	var report struct {
		Findings []doctorFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		s.t.Fatalf("doctor --json is not JSON: %v", err)
	}
	return report.Findings
}

func (s *sandbox) problems() []doctorFinding {
	var out []doctorFinding
	for _, item := range s.doctor() {
		if item.Level == "problem" {
			out = append(out, item)
		}
	}
	return out
}

func findKind(findings []doctorFinding, kind string) *doctorFinding {
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func (r result) ok(t *testing.T) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("jig exited %d\n%s", r.code, r.stderr)
	}
}

func (r result) failed(t *testing.T) {
	t.Helper()
	if r.code == 0 {
		t.Fatalf("jig exited 0, expected a failure\n%s", r.stdout)
	}
}

func (r result) ran(t *testing.T) *probeReport {
	t.Helper()
	if r.probe == nil {
		t.Fatalf("the tool did not run, jig exited %d\n%s", r.code, r.stderr)
	}
	return r.probe
}

func (s *sandbox) wantArgs(want []string, args ...string) {
	s.t.Helper()
	got := s.jig(args...).ran(s.t).Args
	if !slices.Equal(got, want) {
		s.t.Fatalf("argv: want %q, got %q", want, got)
	}
}

func (s *sandbox) wantBlocked(stderrHas string, args ...string) {
	s.t.Helper()
	r := s.jig(args...)
	if r.probe != nil {
		s.t.Fatalf("the tool ran but should not have: argv %q", r.probe.Args)
	}
	r.failed(s.t)
	if !strings.Contains(r.stderr, stderrHas) {
		s.t.Fatalf("stderr lacks %q:\n%s", stderrHas, r.stderr)
	}
}

func contains(t *testing.T, label, text, sub string) {
	t.Helper()
	if !strings.Contains(text, sub) {
		t.Fatalf("%s lacks %q:\n%s", label, sub, text)
	}
}

func lacks(t *testing.T, label, text, sub string) {
	t.Helper()
	if strings.Contains(text, sub) {
		t.Fatalf("%s must not contain %q:\n%s", label, sub, text)
	}
}
