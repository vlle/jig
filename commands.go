package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func cmdList(cfg Config, args []string) error {
	asJSON, args := hasFlag(args, "--json")
	onlyIDs, args := hasFlag(args, "--ids")
	showAll, args := hasFlag(args, "--all")
	kind, args := flagValue(args, "--kind")
	tag, args := flagValue(args, "--tag")
	query, args := flagValue(args, "--search")
	if query == "" && len(args) > 0 {
		query = strings.Join(args, " ")
	}

	result := scan(cfg)

	var tools []Tool
	for _, tool := range result.Tools {
		switch {
		case !showAll && tool.Status == "deprecated":
		case kind == "" && !showAll && tool.Kind != "tool":
		case kind != "" && tool.Kind != kind:
		case tag != "" && !tool.hasTag(tag):
		case !tool.matches(query):
		default:
			tools = append(tools, tool)
		}
	}

	if asJSON {
		if tools == nil {
			tools = []Tool{}
		}
		return emitJSON(tools)
	}
	if onlyIDs {
		for _, tool := range tools {
			fmt.Println(tool.ID)
		}
		return nil
	}

	if len(tools) == 0 {
		fmt.Fprintln(os.Stderr, "nothing found")
		printManifestErrors(result.Errors)
		return nil
	}

	width := 0
	for _, tool := range tools {
		width = max(width, len(tool.ID))
	}
	for _, tool := range tools {
		badge := statusBadge(tool.Status)
		if badge != "" {
			badge = " " + badge
		}
		fmt.Printf("%s%s%s  %s%s\n", color.bold, pad(tool.ID, width), color.reset, tool.Summary, badge)
	}

	fmt.Fprintf(os.Stderr, "\n%d tools · %s\n", len(tools), cfg.Root)
	printManifestErrors(result.Errors)
	return nil
}

func lookupTool(cfg Config, id string, allowPartial bool) (Tool, error) {
	result := scan(cfg)

	for _, tool := range result.Tools {
		if tool.ID == id {
			return tool, nil
		}
	}

	var partial []Tool
	var names []string
	for _, tool := range result.Tools {
		if strings.Contains(tool.ID, id) {
			partial = append(partial, tool)
			names = append(names, tool.ID)
		}
	}

	switch {
	case len(partial) > 0 && !allowPartial:
		return Tool{}, fmt.Errorf("run needs the exact id, %q matches: %s", id, strings.Join(names, ", "))
	case len(partial) == 1:
		return partial[0], nil
	case len(partial) > 1:
		return Tool{}, fmt.Errorf("ambiguous id %q: %s", id, strings.Join(names, ", "))
	}
	return Tool{}, fmt.Errorf("no tool %q, see `jig ls`", id)
}

func cmdShow(cfg Config, args []string) error {
	asJSON, args := hasFlag(args, "--json")
	if len(args) == 0 {
		return errors.New("usage: jig show <id> [--json]")
	}

	tool, err := lookupTool(cfg, args[0], true)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(tool)
	}

	fmt.Printf("%s%s%s — %s\n\n", color.bold, tool.ID, color.reset, tool.Summary)
	if why := strings.TrimSpace(tool.Why); why != "" {
		fmt.Printf("%s\n\n", why)
	}

	for _, field := range toolFields(cfg, tool, safetyBadge(tool.Safety)) {
		fmt.Printf("  %-10s %s\n", field[0], field[1])
	}
	if tool.Branch != "" {
		fmt.Printf("  %-10s %s\n", "branch", tool.Branch)
	}

	if len(tool.Args) > 0 {
		fmt.Printf("\n%sflags%s\n", color.bold, color.reset)
		for _, arg := range tool.Args {
			line := fmt.Sprintf("  %-12s %s", arg.Flag, arg.Desc)
			if arg.Example != "" {
				line += fmt.Sprintf(" %s(e.g. %s)%s", color.dim, arg.Example, color.reset)
			}
			fmt.Println(line)
		}
	}
	return nil
}

func toolFields(cfg Config, tool Tool, safety string) [][2]string {
	fields := [][2]string{
		{"run", tool.Run},
		{"from", relTo(cfg.Root, tool.WorkdirAbs)},
		{"safety", safety},
	}
	if tool.guardsItself() {
		fields = append(fields, [2]string{"guard", guardLabel})
	} else if tool.NeedsConfirm {
		fields = append(fields, [2]string{"confirm", "jig run --yes " + tool.ID})
	}
	optional := [][2]string{
		{"targets", strings.Join(tool.Targets, ", ")},
		{"secrets", tool.Env},
		{"ticket", tool.Ticket},
		{"tags", strings.Join(tool.Tags, ", ")},
		{"path", tool.Path},
		{"manifest", relTo(cfg.Root, tool.ManifestPath)},
	}
	for _, field := range optional {
		if field[1] != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func sourcePath(tool Tool) string {
	if tool.SourceAbs != "" {
		return tool.SourceAbs
	}

	for _, candidate := range []string{"main.go", "run.js", "main.sh", "run.sh", "index.mjs", "main.py"} {
		path := filepath.Join(tool.TargetDir, candidate)
		if fileExists(path) {
			return path
		}
	}

	entries, err := os.ReadDir(tool.TargetDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".go", ".sh", ".js", ".mjs", ".py":
			return filepath.Join(tool.TargetDir, entry.Name())
		}
	}
	return ""
}

func cmdSource(cfg Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: jig src <id>")
	}

	tool, err := lookupTool(cfg, args[0], true)
	if err != nil {
		return err
	}

	path := sourcePath(tool)
	if path == "" {
		return fmt.Errorf("no source found next to %s, set `source` in the manifest", tool.Path)
	}

	fmt.Fprintf(os.Stderr, "%s%s%s\n", color.dim, path, color.reset)
	body, err := highlight(path, 0)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(body)
	return err
}

func cmdRun(cfg Config, args []string, record *runRecord) error {
	own, passthrough := splitPassthrough(args)
	confirmed, own := hasFlag(own, "--yes")
	if len(own) == 0 {
		return errors.New("usage: jig run [--yes] <id> [-- args...]")
	}

	tool, err := lookupTool(cfg, own[0], false)
	if err != nil {
		return err
	}
	extra := slices.Concat(own[1:], passthrough)

	record.Tool = tool.ID
	record.Safety = tool.Safety
	record.Targets = tool.Targets
	record.Run = tool.Run
	record.Workdir = tool.WorkdirAbs

	if tool.Status == "deprecated" {
		fmt.Fprintf(os.Stderr, "%sthis tool is deprecated%s\n", color.yellow, color.reset)
	}
	if tool.NeedsConfirm && !confirmed {
		return fmt.Errorf("safety=%s on %s without guard: self — confirm with `jig run --yes %s`",
			tool.Safety, strings.Join(tool.Targets, ","), tool.ID)
	}
	if tool.Env != "" {
		fmt.Fprintf(os.Stderr, "%ssecrets: profile %s — load it before running%s\n", color.dim, tool.Env, color.reset)
	}

	fmt.Fprintf(os.Stderr, "%s▸ %s%s  %s%s%s\n",
		color.cyan, displayCommand(tool.Run, extra), color.reset, color.dim, relTo(cfg.Root, tool.WorkdirAbs), color.reset)

	command := exec.Command("bash", slices.Concat([]string{"-c", tool.Run + ` "$@"`, tool.ID}, extra)...)
	command.Dir = tool.WorkdirAbs
	command.Env = append(os.Environ(), "JIG_RUN_ID="+record.RunID, "JIG_TOOL="+tool.ID)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	started := time.Now()
	runErr := command.Run()
	elapsed := time.Since(started).Seconds()

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		fmt.Fprintf(os.Stderr, "%s✓ %s · %.1fs%s\n", color.dim, tool.ID, elapsed, color.reset)
		return nil
	case errors.As(runErr, &exitErr):
		fmt.Fprintf(os.Stderr, "%s✗ %s · exit %d · %.1fs%s\n", color.red, tool.ID, exitErr.ExitCode(), elapsed, color.reset)
		return exitError{code: exitErr.ExitCode()}
	}
	return runErr
}

func displayCommand(run string, args []string) string {
	parts := []string{run}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"$`\\;&|<>()*?[]{}~#!") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func highlight(path string, width int) (string, error) {
	if bat, err := exec.LookPath("bat"); err == nil && colorEnabled() {
		args := []string{"--color=always", "--style=plain", "--paging=never"}
		if width > 0 {
			args = append(args, fmt.Sprintf("--terminal-width=%d", width))
		}
		if out, runErr := exec.Command(bat, append(args, path)...).Output(); runErr == nil && len(out) > 0 {
			return string(out), nil
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
