package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/vlle/jig/internal/anim"
)

//go:embed templates
var templates embed.FS

type scaffoldFile struct {
	name     string
	template string
	exec     bool
}

type scaffold struct {
	files  []scaffoldFile
	run    string
	module bool
}

const animFile = "@anim"

var scaffolds = map[string]scaffold{
	"go": {
		files: []scaffoldFile{
			{name: "main.go", template: "templates/go/main.go.tmpl"},
			{name: "screen.go", template: animFile},
			{name: "runlog.go", template: "templates/shared/runlog.go.tmpl"},
		},
		module: true,
	},
	"go-parallel": {
		files: []scaffoldFile{
			{name: "main.go", template: "templates/go-parallel/main.go.tmpl"},
			{name: "screen.go", template: animFile},
			{name: "runlog.go", template: "templates/shared/runlog.go.tmpl"},
		},
		module: true,
	},
	"bash": {
		files: []scaffoldFile{{name: "main.sh", template: "templates/bash/main.sh.tmpl", exec: true}},
		run:   "%s/main.sh",
	},
	"node": {
		files: []scaffoldFile{{name: "run.js", template: "templates/node/run.js.tmpl", exec: true}},
		run:   "node %s/run.js",
	},
}

const (
	scaffoldGo  = "1.24"
	todoSummary = "TODO one line — the problem this tool answers"
	todoWhy     = "TODO what it does and why this way and not another."
)

var (
	toolID  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	envName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

type templateData struct {
	ID         string
	Envs       []string
	DefaultEnv string
	EnvList    string
	EnvCases   string
	EnvPipe    string
	EnvJS      string
}

func cmdNew(cfg Config, args []string) error {
	kind, args := flagValue(args, "--kind")
	dir, args := flagValue(args, "--dir")
	force, args := hasFlag(args, "--force")

	if len(args) != 1 {
		return errors.New("usage: jig new <id> [--kind go|go-parallel|bash|node] [--dir DIR] [--force]")
	}
	id := args[0]
	if !toolID.MatchString(id) {
		return fmt.Errorf("id %q is not valid: lowercase latin letters, digits and dashes", id)
	}

	if kind == "" {
		kind = "go"
	}
	plan, ok := scaffolds[kind]
	if !ok {
		return fmt.Errorf("unknown --kind=%q, expected go, go-parallel, bash or node", kind)
	}

	data, err := newTemplateData(id, cfg.Envs)
	if err != nil {
		return err
	}

	existing := scan(cfg)
	for _, tool := range existing.Tools {
		if tool.ID == id {
			return fmt.Errorf("tool %q already exists: %s", id, tool.ManifestPath)
		}
	}

	if similar := findSimilar(existing.Tools, id); len(similar) > 0 && !force {
		fmt.Fprintf(os.Stderr, "%ssimilar tools already exist:%s\n", color.yellow, color.reset)
		for _, tool := range similar {
			fmt.Fprintf(os.Stderr, "  %s — %s\n", tool.ID, tool.Summary)
		}
		fmt.Fprintf(os.Stderr, "\nextending one of them is usually cheaper than a new script (jig show <id>, jig src <id>).\n")
		fmt.Fprintf(os.Stderr, "create it anyway: jig new %s --force\n", id)
		return errors.New("stopped, nothing was created")
	}

	if dir == "" {
		dir = filepath.Join("scripts", id)
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, inside := within(cfg.Root, target)
	if !inside {
		return fmt.Errorf("directory %s is outside the workspace %s", target, cfg.Root)
	}

	manifestPath := filepath.Join(cfg.Registry, id+".yml")
	if fileExists(manifestPath) && !force {
		return fmt.Errorf("manifest %s already exists", manifestPath)
	}
	if _, statErr := os.Stat(target); statErr == nil && !force {
		return fmt.Errorf("directory %s already exists, pick another --dir or pass --force", target)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	created := make([]string, 0, len(plan.files)+1)
	for _, item := range plan.files {
		path := filepath.Join(target, item.name)
		if err := writeScaffoldFile(item, data, path); err != nil {
			return err
		}
		created = append(created, path)
	}

	if err := os.MkdirAll(cfg.Registry, 0o755); err != nil {
		return err
	}

	manifest := scaffoldManifest(id, filepath.ToSlash(rel), cfg.Envs)
	base := workBase(target)
	if plan.module {
		module := goModuleDir(cfg, target)
		if module == "" {
			path := filepath.Join(target, "go.mod")
			if err := os.WriteFile(path, []byte("module "+id+"\n\ngo "+scaffoldGo+"\n"), 0o644); err != nil {
				return err
			}
			created = append(created, path)
			module = target
		}
		manifest.Workdir, manifest.Run = goRun(base, module, target)
	} else {
		manifest.Run = fmt.Sprintf(plan.run, runPath(base, target))
	}

	if err := os.WriteFile(manifestPath, []byte(manifestYAML(manifest)), 0o644); err != nil {
		return err
	}
	created = append(created, manifestPath)

	fmt.Printf("%screated%s\n", color.green, color.reset)
	for _, path := range created {
		fmt.Printf("  %s\n", relTo(cfg.Root, path))
	}
	fmt.Printf("\nnext: fill in summary and why in the manifest, write the logic, then `jig show %s` and `jig doctor`\n", id)
	return nil
}

func newTemplateData(id string, envs []string) (templateData, error) {
	data := templateData{ID: id, Envs: envs}
	if len(envs) == 0 {
		return data, nil
	}

	quoted := make([]string, 0, len(envs))
	single := make([]string, 0, len(envs))
	for _, env := range envs {
		if !envName.MatchString(env) {
			return data, fmt.Errorf("envs in jig.yml: %q is not a valid environment name", env)
		}
		quoted = append(quoted, fmt.Sprintf("%q", env))
		single = append(single, "'"+env+"'")
	}

	data.DefaultEnv = envs[0]
	data.EnvList = strings.Join(envs, ", ")
	data.EnvCases = strings.Join(quoted, ", ")
	data.EnvPipe = strings.Join(envs, "|")
	data.EnvJS = strings.Join(single, ", ")
	return data, nil
}

func writeScaffoldFile(item scaffoldFile, data templateData, path string) error {
	mode := os.FileMode(0o644)
	if item.exec {
		mode = 0o755
	}

	if item.template == animFile {
		return os.WriteFile(path, []byte(shippedScreen()), mode)
	}

	raw, err := templates.ReadFile(item.template)
	if err != nil {
		return err
	}
	parsed, err := template.New(filepath.Base(item.template)).Parse(string(raw))
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return parsed.Execute(file, data)
}

func shippedScreen() string {
	return strings.Replace(anim.Source, "package anim", "package main", 1)
}

var genericTerms = map[string]bool{"tool": true, "tools": true, "script": true, "scripts": true, "util": true, "utils": true, "helper": true}

func findSimilar(tools []Tool, id string) []Tool {
	terms := idTerms(id)

	var similar []Tool
	for _, tool := range tools {
		candidates := idTerms(tool.ID)
		for _, tag := range tool.Tags {
			candidates = append(candidates, strings.ToLower(tag))
		}
		for _, term := range terms {
			if slices.Contains(candidates, term) {
				similar = append(similar, tool)
				break
			}
		}
	}
	return similar
}

func idTerms(id string) []string {
	var terms []string
	for _, term := range strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' }) {
		if len(term) >= 4 && !genericTerms[term] {
			terms = append(terms, term)
		}
	}
	return terms
}

func scaffoldManifest(id, rel string, envs []string) Manifest {
	target := "local"
	if len(envs) > 0 {
		target = envs[0]
	}
	return Manifest{
		ID:      id,
		Kind:    "tool",
		Path:    rel,
		Summary: todoSummary,
		Safety:  "read-only",
		Targets: []string{target},
		Guard:   "self",
		Why:     todoWhy,
		Status:  "active",
	}
}

func workBase(target string) string {
	if repo := resolveRepo(target, map[string]repoInfo{}); repo.root != "" {
		return repo.root
	}
	return target
}

func runPath(base, target string) string {
	if rel := relTo(base, target); rel != "." {
		return "./" + filepath.ToSlash(rel)
	}
	return "."
}

func goModuleDir(cfg Config, dir string) string {
	stop := resolveRepo(dir, map[string]repoInfo{}).root
	if stop == "" {
		stop = cfg.Root
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		if dir == stop || dir == filepath.Dir(dir) {
			return ""
		}
		dir = filepath.Dir(dir)
	}
}

func goRun(base, module, pkg string) (workdir, run string) {
	if module != base {
		workdir = filepath.ToSlash(relTo(base, module))
	}
	return workdir, "go run " + shellQuote(runPath(module, pkg))
}
