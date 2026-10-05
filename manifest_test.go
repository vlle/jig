package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManifestYAMLRoundTrip(t *testing.T) {
	cases := map[string]Manifest{
		"plain": {ID: "stock-diag", Kind: "tool", Path: "svc/scripts/stock-diag", Summary: "item shows out of stock",
			Run: "go run ./svc/scripts/stock-diag", Safety: "read-only", Targets: []string{"prod"}, Guard: "self",
			Why: "walks every stage\n\n  indented line\n", Tags: []string{"stock", "incident"}, Status: "active"},
		"needs quoting": {ID: "odd", Kind: "env", Path: "a b/c: d", Summary: `"quoted" #hash: colon`,
			Run: "'./a b/run.sh' --x='1'", Workdir: "a b", Safety: "writes", Targets: []string{"yes", "123", "a,b", "with space"},
			Env: "pg-ro", Why: "- looks like a list\n# looks like a comment", Tags: []string{"ünïcode", "-dash", "null"},
			Status: "incident-only", Ticket: "OPS-1"},
		"empty lists": {ID: "bare", Kind: "tool", Path: "x.sh", Summary: "s", Run: "./x.sh", Safety: "destructive", Status: "deprecated"},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "m.yml")
			if err := os.WriteFile(file, []byte(manifestYAML(want)), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := loadManifest(file)
			if err != nil {
				t.Fatalf("%v\n%s", err, manifestYAML(want))
			}
			if got, want = normalized(got), normalized(want); !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed the manifest\nwant %#v\ngot  %#v\n%s", want, got, manifestYAML(want))
			}
		})
	}
}

func normalized(m Manifest) Manifest {
	if m.Why != "" {
		m.Why = strings.TrimRight(m.Why, "\n ") + "\n"
	}
	if len(m.Targets) == 0 {
		m.Targets = nil
	}
	if len(m.Tags) == 0 {
		m.Tags = nil
	}
	return m
}
