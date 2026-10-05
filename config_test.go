package main

import (
	"path/filepath"
	"testing"
)

func TestSkipped(t *testing.T) {
	cfg := Config{Root: "/ws", Skip: []string{"play*", "/legacy", "svc/gen"}}
	cases := map[string]bool{
		"/ws":                 false,
		"/ws/.cache":          true,
		"/ws/svc/vendor":      true,
		"/ws/play3":           true,
		"/ws/a/play-old":      true,
		"/ws/legacy":          true,
		"/ws/svc/legacy":      false,
		"/ws/svc/gen":         true,
		"/ws/other/svc/gen":   false,
		"/ws/svc/scripts/foo": false,
	}
	for dir, want := range cases {
		if got := cfg.skipped(filepath.FromSlash(dir)); got != want {
			t.Errorf("skipped(%s) = %v, want %v", dir, got, want)
		}
	}
}

func TestRedactArgs(t *testing.T) {
	got := redactArgs([]string{"-token", "abc", "--api-key=xyz", "-n", "5", "-password"})
	want := []string{"-token", "***", "--api-key=***", "-n", "5", "-password"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestFindSimilar(t *testing.T) {
	tools := []Tool{
		{Manifest: Manifest{ID: "stock-diag", Tags: []string{"inventory"}}},
		{Manifest: Manifest{ID: "sh-tool"}},
	}
	cases := map[string]int{
		"stock-recount":  1,
		"inventory-sync": 1,
		"js-tool":        0,
		"orders-export":  0,
	}
	for id, want := range cases {
		if got := len(findSimilar(tools, id)); got != want {
			t.Errorf("findSimilar(%s) = %d, want %d", id, got, want)
		}
	}
}

func TestLetterHopIsStaggered(t *testing.T) {
	hops := 0
	for tick := range hopCycle {
		up := 0
		for letter := range logoGlyphs {
			up += letterHop(letter, tick)
		}
		if up > 1 {
			t.Fatalf("tick %d: %d letters in the air at once", tick, up)
		}
		hops += up
	}
	if hops != len(logoGlyphs)*hopDuration {
		t.Fatalf("each letter must hop once per cycle, got %d airborne ticks", hops)
	}
}

func TestMissingRunTargetAcceptsGoPatterns(t *testing.T) {
	dir := t.TempDir()
	for run, want := range map[string]string{
		"go test ./...":      "",
		"go run ./cmd/x/...": "./cmd/x/...",
		"./missing.sh":       "./missing.sh",
		"go run .":           "",
	} {
		if got := missingRunTarget(Tool{Manifest: Manifest{Run: run}, WorkdirAbs: dir}); got != want {
			t.Errorf("missingRunTarget(%q) = %q, want %q", run, got, want)
		}
	}
}
