package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeScript(t *testing.T) {
	cases := []struct {
		name, file, body, summary, about string
	}{
		{
			name:    "shell header after shebang and shellcheck",
			file:    "check-stock.sh",
			body:    "#!/usr/bin/env bash\n# shellcheck disable=SC2086\n# Tells why a published item shows out of stock. Reads prod.\n# Second line of the header.\n\nset -e\n",
			summary: "Tells why a published item shows out of stock",
			about:   "Tells why a published item shows out of stock. Reads prod.\nSecond line of the header.",
		},
		{
			name:    "comment after a prelude",
			file:    "sync.sh",
			body:    "#!/bin/bash\nset -euo pipefail\nIFS=$'\\n\\t'\n\n# Copies the nightly dump to the replica\necho go\n",
			summary: "Copies the nightly dump to the replica",
		},
		{
			name:    "license paragraph is skipped",
			file:    "rotate.sh",
			body:    "#!/bin/sh\n#\n# Copyright 2024 Example\n# Licensed under MIT, see the file\n#\n# rotate.sh — rotates the signing key in the vault\n",
			summary: "rotates the signing key in the vault",
		},
		{
			name:    "decoration lines are separators",
			file:    "report.sh",
			body:    "#!/bin/sh\n##########\n# weekly revenue report\n##########\nmain() { :; }\n",
			summary: "weekly revenue report",
		},
		{
			name:    "a code comment below the code is not a header",
			file:    "loop.sh",
			body:    "#!/bin/sh\nfor x in a b; do\n  # loops over shards\n  echo $x\ndone\n",
			summary: "",
		},
		{
			name:    "python docstring",
			file:    "count.py",
			body:    "#!/usr/bin/env python3\n# -*- coding: utf-8 -*-\n\"\"\"Counts orders stuck in the payment queue.\n\nMore details here.\n\"\"\"\nimport sys\n",
			summary: "Counts orders stuck in the payment queue",
		},
		{
			name:    "python argparse description",
			file:    "export.py",
			body:    "import argparse\n\nparser = argparse.ArgumentParser(description=\"Exports invoices to CSV\")\n",
			summary: "Exports invoices to CSV",
		},
		{
			name:    "go doc comment before package main",
			file:    "main.go",
			body:    "//go:build tools\n\n// Command stock-diag reports items that are published but out of stock.\n// It reads every shard.\npackage main\n",
			summary: "stock-diag reports items that are published but out of stock",
		},
		{
			name:    "js block comment after shebang and use strict",
			file:    "run.js",
			body:    "#!/usr/bin/env node\n'use strict';\n/**\n * Warms the CDN cache for the top pages.\n */\nconst x = 1;\n",
			summary: "Warms the CDN cache for the top pages",
		},
		{
			name:    "usage line as a fallback",
			file:    "deploy.sh",
			body:    "#!/bin/sh\nif [ -z \"$1\" ]; then\n  echo \"usage: $0 <service> [--force]\" >&2\n  exit 2\nfi\n",
			summary: "usage: deploy.sh <service> [--force]",
		},
		{
			name:    "nothing to say",
			file:    "bare.sh",
			body:    "#!/bin/sh\necho hi\n",
			summary: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			summary, about := describeScript(path)
			if summary != tc.summary {
				t.Fatalf("summary: want %q, got %q", tc.summary, summary)
			}
			if tc.about != "" && about != tc.about {
				t.Fatalf("about: want %q, got %q", tc.about, about)
			}
		})
	}
}

func TestSentence(t *testing.T) {
	cases := map[string]string{
		"gofmt -w .":                 "gofmt -w .",
		"Builds the release binary.": "Builds the release binary",
		"short. Second":              "short. Second",
		"Builds the binary for all platforms. Then uploads it.": "Builds the binary for all platforms",
	}
	for in, want := range cases {
		if got := sentence(in); got != want {
			t.Errorf("sentence(%q) = %q, want %q", in, got, want)
		}
	}

	long := sentence(strings.Repeat("word ", 40))
	if !strings.HasSuffix(long, "…") || len([]rune(long)) > summaryLength+1 {
		t.Fatalf("long text is not clipped: %q", long)
	}
}

func TestDropLeadingName(t *testing.T) {
	cases := map[string]string{
		"build       — собрать все бинари": "собрать все бинари",
		"Build: the release": "the release",
		"builder of things":  "builder of things",
		"build":              "build",
	}
	for in, want := range cases {
		if got := dropLeadingName(in, "build"); got != want {
			t.Errorf("dropLeadingName(%q) = %q, want %q", in, got, want)
		}
	}
}
