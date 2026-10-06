package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	originRegistry = "registry"
	originScript   = "script"
)

var originLabels = map[string]string{
	originScript: "script without a manifest",
	"make":       "make target",
	"just":       "just recipe",
	"npm":        "package.json script",
	"task":       "Taskfile task",
}

type workspaceFiles struct {
	scripts []string
	runners []string
	empties []string
	files   int
}

const walkWorkers = 16

func walkWorkspace(cfg Config, progress func(int)) workspaceFiles {
	var (
		ws      workspaceFiles
		mu      sync.Mutex
		pending sync.WaitGroup
		slots   = make(chan struct{}, walkWorkers)
	)

	var visit func(dir string)
	visit = func(dir string) {
		defer pending.Done()
		slots <- struct{}{}
		entries, err := os.ReadDir(dir)
		<-slots
		if err != nil {
			return
		}

		var local workspaceFiles
		var subdirs []string
		if len(entries) == 0 && dir != cfg.Root && cfg.inToolDir(dir) {
			local.empties = append(local.empties, dir)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if !cfg.skipped(path) && !bundledDirs[entry.Name()] && path != cfg.Registry {
					subdirs = append(subdirs, path)
				}
				continue
			}
			local.files++
			switch {
			case isScriptCandidate(cfg, path):
				local.scripts = append(local.scripts, path)
			case runnerFiles[entry.Name()] != "":
				local.runners = append(local.runners, path)
			}
		}

		mu.Lock()
		before := ws.files
		ws.files += local.files
		ws.scripts = append(ws.scripts, local.scripts...)
		ws.runners = append(ws.runners, local.runners...)
		ws.empties = append(ws.empties, local.empties...)
		if progress != nil && ws.files/250 != before/250 {
			progress(ws.files)
		}
		mu.Unlock()

		pending.Add(len(subdirs))
		for _, sub := range subdirs {
			go visit(sub)
		}
	}

	pending.Add(1)
	go visit(cfg.Root)
	pending.Wait()

	sort.Strings(ws.scripts)
	sort.Strings(ws.runners)
	sort.Strings(ws.empties)
	if progress != nil {
		progress(ws.files)
	}
	return ws
}

func uncovered(tools []Tool, scripts []string) []string {
	covered := map[string]bool{}
	coveredDirs := map[string]bool{}
	for _, tool := range tools {
		stat, err := os.Stat(tool.TargetAbs)
		if err != nil {
			continue
		}
		if stat.IsDir() {
			coveredDirs[tool.TargetAbs] = true
		}
		if path := sourcePath(tool); path != "" {
			covered[path] = true
		}
	}

	var out []string
	for _, path := range scripts {
		if !covered[path] && !coveredDirs[filepath.Dir(path)] {
			out = append(out, path)
		}
	}
	return out
}

func discover(cfg Config, registered []Tool) []Tool {
	ws := walkWorkspace(cfg, nil)
	repos := map[string]repoInfo{}
	scripts := dedupeByCheckout(dropIgnored(cfg.Root, ws.scripts), repos)
	runners := dedupeByCheckout(dropIgnored(cfg.Root, ws.runners), repos)

	found := foundScripts(cfg, uncovered(registered, scripts), repos)
	for _, file := range runners {
		found = append(found, foundRunners(cfg, file, repos)...)
	}

	taken := map[string]bool{}
	for _, tool := range registered {
		taken[tool.ID] = true
	}
	kept := found[:0]
	for _, tool := range found {
		if !taken[tool.ID] {
			kept = append(kept, tool)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].ID < kept[j].ID })
	return kept
}

func foundScripts(cfg Config, paths []string, repos map[string]repoInfo) []Tool {
	var tools []Tool
	goDirs := map[string][]string{}
	for _, path := range paths {
		switch filepath.Ext(path) {
		case ".go":
			goDirs[filepath.Dir(path)] = append(goDirs[filepath.Dir(path)], path)
			continue
		case ".py":
			if !runnablePython(path) {
				continue
			}
		case ".js", ".mjs":
			if !runnableNode(path) {
				continue
			}
		}
		summary, about := describeScript(path)
		if tool, ok := foundScript(cfg, path, summary, about, repos); ok {
			tools = append(tools, tool)
		}
	}

	for dir, files := range goDirs {
		summary, about, isMain := describeGoPackage(files)
		if !isMain {
			continue
		}
		if tool, ok := foundScript(cfg, dir, summary, about, repos); ok {
			tools = append(tools, tool)
		}
	}
	return tools
}

func foundScript(cfg Config, target, summary, about string, repos map[string]repoInfo) (Tool, bool) {
	rel := filepath.ToSlash(relTo(cfg.Root, target))
	manifest := Manifest{ID: rel, Kind: "tool", Path: rel, Summary: summary, Why: about, Safety: "writes", Status: "active"}

	workdir, run, err := guessRun(cfg, resolveTool(cfg, manifest, "", repos))
	if err != nil {
		return Tool{}, false
	}
	manifest.Workdir, manifest.Run = workdir, run

	tool := resolveTool(cfg, manifest, "", repos)
	tool.Origin = originScript
	return tool, true
}

func foundRunners(cfg Config, file string, repos map[string]repoInfo) []Tool {
	origin := runnerFiles[filepath.Base(file)]
	rel := filepath.ToSlash(relTo(cfg.Root, file))
	dir := filepath.Dir(file)
	base := resolveRepo(dir, repos).root
	if base == "" {
		base = dir
	}
	workdir := ""
	if dir != base {
		workdir = filepath.ToSlash(relTo(base, dir))
	}

	var tools []Tool
	for _, item := range readRunners(file, cfg.Root) {
		manifest := Manifest{ID: rel + ":" + item.name, Kind: "tool", Path: rel, Summary: item.summary,
			Run: item.run, Workdir: workdir, Safety: "writes", Status: "active"}
		tool := resolveTool(cfg, manifest, "", repos)
		tool.Origin = origin
		tools = append(tools, tool)
	}
	return tools
}

func runnablePython(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(raw)
	return strings.HasPrefix(text, "#!") || strings.Contains(text, "__name__") && strings.Contains(text, "__main__")
}

func runnableNode(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(raw)
	return strings.HasPrefix(text, "#!") || strings.Contains(text, "process.") && !strings.Contains(text, "document.")
}

func describeGoPackage(files []string) (summary, about string, isMain bool) {
	sort.Slice(files, func(i, j int) bool {
		iMain, jMain := filepath.Base(files[i]) == "main.go", filepath.Base(files[j]) == "main.go"
		if iMain != jMain {
			return iMain
		}
		return files[i] < files[j]
	})

	for _, file := range files {
		if !declaresMain(file) {
			continue
		}
		isMain = true
		if summary, about = describeScript(file); summary != "" {
			return summary, about, true
		}
	}
	return summary, about, isMain
}

func declaresMain(file string) bool {
	for _, line := range headOf(file, headLimit) {
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "package" {
			return fields[1] == "main"
		}
	}
	return false
}

func foundNote(tool Tool) string {
	if tool.Origin == originScript {
		return "not registered — `jig add " + shellQuote(tool.Path) + "` keeps it in the registry"
	}
	return "not registered — " + originLabels[tool.Origin] + " in " + tool.Path
}
