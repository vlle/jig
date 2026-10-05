package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const addUsage = "usage: jig add <path>... [--id ID] [--summary TEXT] [--why TEXT] [--safety read-only|writes|destructive] [--targets a,b] [--tags a,b] [--run CMD]"

var idJunk = regexp.MustCompile(`[^a-z0-9]+`)

type addOptions struct {
	id, summary, why, run, safety string
	targets, tags                 []string
}

func cmdAdd(cfg Config, args []string) error {
	var opts addOptions
	var targets, tags string
	opts.id, args = flagValue(args, "--id")
	opts.summary, args = flagValue(args, "--summary")
	opts.why, args = flagValue(args, "--why")
	opts.run, args = flagValue(args, "--run")
	opts.safety, args = flagValue(args, "--safety")
	targets, args = flagValue(args, "--targets")
	tags, args = flagValue(args, "--tags")
	opts.targets, opts.tags = splitList(targets), splitList(tags)

	if len(args) == 0 {
		return errors.New(addUsage)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown flag %s\n%s", arg, addUsage)
		}
	}
	if len(args) > 1 && (opts.id != "" || opts.summary != "" || opts.why != "" || opts.run != "") {
		return errors.New("--id, --summary, --why and --run describe one script, pass a single path")
	}

	existing := scan(cfg)
	taken := map[string]string{}
	for _, tool := range existing.Tools {
		taken[tool.ID] = relTo(cfg.Root, tool.ManifestPath)
	}

	var planned []Manifest
	var problems []string
	for _, path := range args {
		manifest, err := planAdd(cfg, existing.Tools, path, opts)
		if err == nil {
			if previous, clash := taken[manifest.ID]; clash {
				err = fmt.Errorf("%s: id %q is already taken by %s, pass --id", path, manifest.ID, previous)
			} else if file := manifestFile(cfg, manifest.ID); fileExists(file) {
				err = fmt.Errorf("%s: %s already exists, pass --id", path, relTo(cfg.Root, file))
			}
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		taken[manifest.ID] = path
		planned = append(planned, manifest)
	}

	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "%s✗%s %s\n", color.red, color.reset, problem)
		}
		return errors.New("nothing was registered")
	}

	if err := os.MkdirAll(cfg.Registry, 0o755); err != nil {
		return err
	}
	unfinished := false
	for _, manifest := range planned {
		file := manifestFile(cfg, manifest.ID)
		if err := os.WriteFile(file, []byte(manifestYAML(manifest)), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s✓%s %s%s%s  %s\n", color.green, color.reset, color.bold, manifest.ID, color.reset, relTo(cfg.Root, file))
		fmt.Printf("  %srun: %s%s\n", color.dim, manifest.Run, color.reset)
		unfinished = unfinished || len(placeholders(manifest)) > 0
	}

	if unfinished {
		fmt.Println("\nnext: replace the TODOs in the manifest, check safety and targets, then `jig doctor` and `jig index`")
	} else {
		fmt.Println("\nnext: `jig doctor` and `jig index`")
	}
	return nil
}

func planAdd(cfg Config, tools []Tool, path string, opts addOptions) (Manifest, error) {
	target, err := filepath.Abs(path)
	if err != nil {
		return Manifest{}, err
	}
	stat, err := os.Stat(target)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s does not exist", path)
	}
	rel, inside := within(cfg.Root, target)
	if !inside || rel == "." {
		return Manifest{}, fmt.Errorf("%s is not inside the workspace %s", path, cfg.Root)
	}
	if _, inRegistry := within(cfg.Registry, target); inRegistry {
		return Manifest{}, fmt.Errorf("%s is inside the registry", path)
	}
	if owner := coveringTool(tools, target); owner != "" {
		return Manifest{}, fmt.Errorf("%s is already registered as %s", path, owner)
	}

	id := opts.id
	if id == "" {
		id = deriveID(target, stat.IsDir())
	}
	if !toolID.MatchString(id) {
		return Manifest{}, fmt.Errorf("%s: id %q is not valid (lowercase latin letters, digits and dashes), pass --id", path, id)
	}

	manifest := Manifest{
		ID:      id,
		Kind:    "tool",
		Path:    filepath.ToSlash(rel),
		Summary: firstNonEmpty(opts.summary, todoSummary),
		Run:     opts.run,
		Safety:  firstNonEmpty(opts.safety, "writes"),
		Targets: opts.targets,
		Why:     firstNonEmpty(opts.why, todoWhy),
		Tags:    opts.tags,
		Status:  "active",
	}
	if manifest.Run == "" {
		manifest.Workdir, manifest.Run, err = guessRun(cfg, resolveTool(cfg, manifest, "", map[string]repoInfo{}))
		if err != nil {
			return Manifest{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	return manifest, manifest.validate(path)
}

func coveringTool(tools []Tool, target string) string {
	for _, tool := range tools {
		if tool.TargetAbs == target {
			return tool.ID
		}
		if _, inside := within(tool.TargetAbs, target); inside && tool.SourceAbs != tool.TargetAbs {
			return tool.ID
		}
	}
	return ""
}

func deriveID(target string, isDir bool) string {
	name := filepath.Base(target)
	if !isDir {
		name = strings.TrimSuffix(name, filepath.Ext(name))
		if name == "main" || name == "run" || name == "index" {
			name = filepath.Base(filepath.Dir(target))
		}
	}
	return strings.Trim(idJunk.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func guessRun(cfg Config, tool Tool) (workdir, run string, err error) {
	entry := sourcePath(tool)
	if entry == "" {
		return "", "", errors.New("no .go, .sh, .js, .mjs or .py file found, pass --run")
	}
	script := runPath(tool.WorkdirAbs, entry)

	switch filepath.Ext(entry) {
	case ".go":
		pkg := filepath.Dir(entry)
		if entry == tool.TargetAbs {
			pkg = entry
		}
		if module := goModuleDir(cfg, filepath.Dir(entry)); module != "" {
			workdir, run = goRun(tool.WorkdirAbs, module, pkg)
			return workdir, run, nil
		}
		return "", "go run " + shellQuote(runPath(tool.WorkdirAbs, pkg)), nil
	case ".py":
		return "", "python3 " + shellQuote(script), nil
	case ".js", ".mjs", ".cjs":
		return "", "node " + shellQuote(script), nil
	}

	if executable(entry) {
		return "", shellQuote(script), nil
	}
	if ext := filepath.Ext(entry); ext == ".sh" || ext == ".bash" {
		return "", "bash " + shellQuote(script), nil
	}
	return "", "", fmt.Errorf("cannot tell how to run %s, pass --run", filepath.Base(entry))
}

func executable(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.Mode().Perm()&0o111 != 0
}

func manifestFile(cfg Config, id string) string {
	return filepath.Join(cfg.Registry, id+".yml")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
