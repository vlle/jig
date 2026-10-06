package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWalkWorkspace(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.sh", "Makefile", "README.md",
		"svc/scripts/x.go", "svc/scripts/y.py", "svc/internal/z.go",
		"svc/web/package.json", "svc/node_modules/dep/package.json",
		".hidden/h.sh", "legacy/old.sh", "deep/1/2/3/4/run.sh",
	}
	for _, rel := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "tools", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.Root = root
	cfg.Skip = []string{"/legacy"}
	cfg.Registry = filepath.Join(root, cfg.Registry)

	var calls int
	ws := walkWorkspace(cfg, func(int) { calls++ })

	rel := func(paths []string) []string {
		out := make([]string, 0, len(paths))
		for _, path := range paths {
			out = append(out, filepath.ToSlash(relTo(root, path)))
		}
		return out
	}
	if got, want := rel(ws.scripts), []string{"a.sh", "deep/1/2/3/4/run.sh", "svc/scripts/x.go", "svc/scripts/y.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("scripts: want %v, got %v", want, got)
	}
	if got, want := rel(ws.runners), []string{"Makefile", "svc/web/package.json"}; !reflect.DeepEqual(got, want) {
		t.Errorf("runners: want %v, got %v", want, got)
	}
	if got, want := rel(ws.empties), []string{"tools/empty"}; !reflect.DeepEqual(got, want) {
		t.Errorf("empties: want %v, got %v", want, got)
	}
	if ws.files != 8 {
		t.Errorf("files: want 8, got %d", ws.files)
	}
	if calls == 0 {
		t.Error("progress was never reported")
	}
}
