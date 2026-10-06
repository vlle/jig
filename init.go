package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdInit(args []string) error {
	dir := "."
	if len(args) > 1 {
		return errors.New("usage: jig init [DIR]")
	}
	if len(args) == 1 {
		dir = args[0]
	}

	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if stat, statErr := os.Stat(root); statErr != nil || !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}

	file := filepath.Join(root, configName)
	if fileExists(file) {
		return fmt.Errorf("%s already exists", file)
	}
	if err := os.WriteFile(file, []byte(starterConfig), 0o644); err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Registry, 0o755); err != nil {
		return err
	}

	fmt.Printf("%screated%s\n  %s\n  %s/\n\n", color.green, color.reset, relTo(root, file), relTo(root, cfg.Registry))

	s, err := newScreen("init")
	if err != nil {
		return err
	}
	var found []Tool
	_ = s.Step("looking for scripts and tasks that already exist", func(update func(string)) error {
		found = discover(cfg, nil)
		update(fmt.Sprintf("%d found", len(found)))
		return nil
	})
	s.Close()

	fmt.Println()
	if summary := foundSummary(found); summary != "" {
		fmt.Printf("found %s — searchable now: `jig ls <words>`\n", summary)
		fmt.Println("register the scripts worth keeping with `jig add <path>`; `jig doctor` lists the ones without a manifest")
	}
	fmt.Println("next: `jig new <id>` for a new tool, `jig index` for TOOLS.md, `jig agent rules` for your coding agent")
	return nil
}

func foundSummary(found []Tool) string {
	counts := map[string]int{}
	described := 0
	for _, tool := range found {
		counts[tool.Origin]++
		if tool.Origin == originScript && tool.Summary != "" {
			described++
		}
	}

	var parts []string
	if n := counts[originScript]; n > 0 {
		parts = append(parts, fmt.Sprintf("%s (%d with a description)", plural(n, "script"), described))
	}
	for _, origin := range []string{"make", "just", "npm", "task"} {
		if n := counts[origin]; n > 0 {
			parts = append(parts, plural(n, originLabels[origin]))
		}
	}
	return strings.Join(parts, ", ")
}
