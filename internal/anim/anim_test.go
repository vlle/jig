package anim

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\033[31mred\033[0m":       "red",
		"\033[?25lhidden\033[?25h": "hidden",
		"plain ✓":                  "plain ✓",
		"\033[2m\033[1mx\033[0m":   "x",
	}
	for input, want := range cases {
		if got := StripANSI(input); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFitCountsVisibleRunes(t *testing.T) {
	line := "\033[32m✓\033[0m shard-1 ready"
	got := Fit(line, 8)
	if visible := StripANSI(got); visible != "✓ shard" {
		t.Fatalf("visible part %q", visible)
	}
	if !strings.HasSuffix(got, "\033[0m") {
		t.Fatalf("a truncated line must end with a reset: %q", got)
	}
	if Fit(line, 0) != line {
		t.Fatal("width 0 must leave the line alone")
	}
}

func TestBar(t *testing.T) {
	s := &Screen{}
	if got := StripANSI(s.Bar(5, 10, 10)); got != "▕█████░░░░░▏  50%" {
		t.Fatalf("bar %q", got)
	}
	if s.Bar(1, 0, 10) != "" {
		t.Fatal("zero total must render nothing")
	}
}

func TestPlainStepPrintsOneLine(t *testing.T) {
	out := capture(t, func(s *Screen) {
		_ = s.Step("first", func(report func(string)) error {
			report("detail")
			return nil
		})
		_ = s.Step("second", func(func(string)) error { return errors.New("boom") })
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", out)
	}
	if !strings.HasPrefix(lines[0], "✓ first   detail") || !strings.HasPrefix(lines[1], "✗ second   boom") {
		t.Fatalf("got %q", lines)
	}
	if strings.Contains(out, "\033[") {
		t.Fatalf("plain output carries escape codes: %q", out)
	}
}

func capture(t *testing.T, fn func(*Screen)) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewScreen(Options{Mode: "never"})
	if err != nil {
		t.Fatal(err)
	}
	s.out = writer
	fn(s)
	_ = writer.Close()
	raw, _ := io.ReadAll(reader)
	return string(raw)
}
