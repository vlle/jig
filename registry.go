package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type repoInfo struct {
	root   string
	remote string
	branch string
}

type scanResult struct {
	Tools  []Tool
	Errors []string
}

func scan(cfg Config) scanResult {
	var result scanResult

	entries, err := os.ReadDir(cfg.Registry)
	if err != nil {
		result.Errors = append(result.Errors, "registry is not readable: "+err.Error())
		return result
	}

	repos := map[string]repoInfo{}
	seen := map[string]string{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(cfg.Registry, entry.Name())
		manifest, loadErr := loadManifest(path)
		if loadErr != nil {
			result.Errors = append(result.Errors, loadErr.Error())
			continue
		}
		if previous, clash := seen[manifest.ID]; clash {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: id %q is already taken by %s", path, manifest.ID, previous))
			continue
		}
		seen[manifest.ID] = path

		tool := resolveTool(cfg, manifest, path, repos)
		tool.Registered, tool.Origin = true, originRegistry
		result.Tools = append(result.Tools, tool)
	}

	sort.Slice(result.Tools, func(i, j int) bool { return result.Tools[i].ID < result.Tools[j].ID })
	return result
}

func resolveTool(cfg Config, manifest Manifest, manifestPath string, repos map[string]repoInfo) Tool {
	tool := Tool{Manifest: manifest, ManifestPath: manifestPath}
	tool.TargetAbs = cfg.resolve(manifest.Path)
	tool.NeedsConfirm = manifest.Safety != "read-only" && cfg.prod(manifest.Targets) && manifest.Guard != "self"

	targetDir := tool.TargetAbs
	if stat, err := os.Stat(tool.TargetAbs); err == nil && !stat.IsDir() {
		targetDir = filepath.Dir(tool.TargetAbs)
		tool.SourceAbs = tool.TargetAbs
	}
	tool.TargetDir = targetDir

	repo := resolveRepo(targetDir, repos)
	tool.RepoRoot = repo.root
	tool.RepoRemote = repo.remote
	tool.Branch = repo.branch
	if repo.root != "" {
		tool.RepoRel = filepath.ToSlash(relTo(repo.root, tool.TargetAbs))
	}

	base := repo.root
	if base == "" {
		base = targetDir
	}
	tool.WorkdirAbs = base
	if manifest.Workdir != "" {
		tool.WorkdirAbs = filepath.Join(base, manifest.Workdir)
	}

	if manifest.Source != "" {
		tool.SourceAbs = filepath.Join(targetDir, manifest.Source)
	}

	return tool
}

func isTrunk(branch string) bool {
	return branch == "master" || branch == "main"
}

func resolveRepo(dir string, cache map[string]repoInfo) repoInfo {
	if info, ok := cache[dir]; ok {
		return info
	}

	gitPath := filepath.Join(dir, ".git")
	if stat, err := os.Stat(gitPath); err == nil {
		gitDir := gitPath
		if !stat.IsDir() {
			gitDir = resolveGitFile(gitPath)
		}
		info := repoInfo{root: dir, remote: readRemote(gitDir), branch: readBranch(gitDir)}
		cache[dir] = info
		return info
	}

	parent := filepath.Dir(dir)
	if parent == dir {
		cache[dir] = repoInfo{}
		return repoInfo{}
	}

	info := resolveRepo(parent, cache)
	cache[dir] = info
	return info
}

func resolveGitFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "gitdir:"))
	if value == "" {
		return ""
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(filepath.Dir(path), value)
	}
	return value
}

func readRemote(gitDir string) string {
	if gitDir == "" {
		return ""
	}
	config := filepath.Join(gitDir, "config")
	if _, err := os.Stat(config); err != nil {
		if common, readErr := os.ReadFile(filepath.Join(gitDir, "commondir")); readErr == nil {
			config = filepath.Join(gitDir, strings.TrimSpace(string(common)), "config")
		}
	}

	file, err := os.Open(config)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	inOrigin := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if inOrigin && strings.HasPrefix(line, "url") {
			if _, value, found := strings.Cut(line, "="); found {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func readBranch(gitDir string) string {
	if gitDir == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	if ref, found := strings.CutPrefix(strings.TrimSpace(string(raw)), "ref: refs/heads/"); found {
		return ref
	}
	return ""
}
