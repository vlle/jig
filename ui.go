package main

import (
	"os"
	"strings"
)

type palette struct {
	reset, bold, dim, red, green, yellow, cyan string
}

var color = newPalette()

func newPalette() palette {
	if !colorEnabled() {
		return palette{}
	}
	return fullPalette()
}

func fullPalette() palette {
	return palette{
		reset:  "\033[0m",
		bold:   "\033[1m",
		dim:    "\033[2m",
		red:    "\033[31m",
		green:  "\033[32m",
		yellow: "\033[33m",
		cyan:   "\033[36m",
	}
}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	stat, err := os.Stdout.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

func safetyBadge(safety string) string {
	switch safety {
	case "read-only":
		return color.green + safety + color.reset
	case "writes":
		return color.yellow + safety + color.reset
	case "destructive":
		return color.red + safety + color.reset
	}
	return safety
}

const guardLabel = "the tool guards itself (guard: self), jig does not ask for --yes"

func statusBadge(status string) string {
	switch status {
	case "deprecated":
		return color.dim + "deprecated" + color.reset
	case "incident-only":
		return color.yellow + "incident-only" + color.reset
	}
	return ""
}

func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}
