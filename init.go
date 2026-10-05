package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	var report doctorReport
	_ = s.Step("looking for scripts that already exist", func(update func(string)) error {
		report = diagnose(cfg, func(files int) { update(fmt.Sprintf("%d files", files)) })
		return nil
	})
	s.Close()

	orphans := 0
	for _, item := range report.Findings {
		if item.Kind == "script without manifest" {
			orphans = len(item.Paths)
		}
	}

	fmt.Println()
	if orphans > 0 {
		noun := "scripts have"
		if orphans == 1 {
			noun = "script has"
		}
		fmt.Printf("%d %s no manifest yet — `jig doctor` lists them, `jig add <path>` registers the ones worth keeping.\n", orphans, noun)
	}
	fmt.Println("next: `jig new <id>` for a new tool, `jig index` for TOOLS.md, `jig agent rules` for your coding agent")
	return nil
}
