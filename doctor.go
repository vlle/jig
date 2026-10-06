package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vlle/jig/internal/anim"
)

type finding struct {
	Kind   string   `json:"kind"`
	Level  string   `json:"level"`
	Detail string   `json:"detail"`
	Paths  []string `json:"paths,omitempty"`
	Fix    string   `json:"fix,omitempty"`
}

type doctorReport struct {
	Root     string    `json:"root"`
	Tools    int       `json:"tools"`
	Files    int       `json:"files"`
	Findings []finding `json:"findings"`
}

const (
	levelProblem = "problem"
	levelInfo    = "info"
	ignoreFile   = ".jigignore"
)

var (
	alwaysScript = map[string]bool{".sh": true, ".bash": true}
	toolDirExts  = map[string]bool{".sh": true, ".bash": true, ".go": true, ".js": true, ".mjs": true, ".py": true}
	bundledDirs  = map[string]bool{"_dist": true, "public": true, "static": true, "assets": true, "coverage": true}
)

func (r doctorReport) problems() int {
	count := 0
	for _, item := range r.Findings {
		if item.Level == levelProblem {
			count++
		}
	}
	return count
}

func cmdDoctor(cfg Config, args []string) error {
	asJSON, _ := hasFlag(args, "--json")

	if asJSON {
		report := diagnose(cfg, nil)
		if report.Findings == nil {
			report.Findings = []finding{}
		}
		if err := emitJSON(report); err != nil {
			return err
		}
		return report.verdict()
	}

	s, err := newScreen("doctor")
	if err != nil {
		return err
	}

	var report doctorReport
	_ = s.Step("scanning "+displayRoot(cfg.Root), func(update func(string)) error {
		report = diagnose(cfg, func(files int) { update(fmt.Sprintf("%d files", files)) })
		return nil
	})
	s.Close()

	writeDoctor(os.Stdout, report, color)
	return report.verdict()
}

func (r doctorReport) verdict() error {
	if r.problems() > 0 {
		return exitError{code: 1}
	}
	return nil
}

func displayRoot(root string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rest, found := strings.CutPrefix(root, home); found {
			return "~" + rest
		}
	}
	return root
}

func diagnose(cfg Config, progress func(files int)) doctorReport {
	result := scan(cfg)
	report := doctorReport{Root: cfg.Root, Tools: len(result.Tools)}
	add := func(item finding) {
		if item.Level == "" {
			item.Level = levelProblem
		}
		report.Findings = append(report.Findings, item)
	}

	for _, message := range result.Errors {
		add(finding{Kind: "broken manifest", Detail: message})
	}

	var unfinished []string
	for _, tool := range result.Tools {
		if _, err := os.Stat(tool.TargetAbs); err != nil {
			add(finding{Kind: "path does not exist", Detail: tool.ID + ": " + tool.Path, Paths: []string{tool.ManifestPath}})
			continue
		}
		if sourcePath(tool) == "" {
			add(finding{Kind: "manifest without source", Detail: tool.ID, Paths: []string{tool.ManifestPath}})
		}
		if target := missingRunTarget(tool); target != "" {
			add(finding{Kind: "run target not found from workdir", Detail: tool.ID + ": " + tool.Run, Paths: []string{tool.WorkdirAbs}})
		}
		if todo := placeholders(tool.Manifest); len(todo) > 0 {
			unfinished = append(unfinished, relTo(cfg.Root, tool.ManifestPath)+": "+strings.Join(todo, ", "))
		}
	}
	if len(unfinished) > 0 {
		add(finding{
			Kind:   "unfinished manifest",
			Detail: fmt.Sprintf("%d — summary or why is still TODO", len(unfinished)),
			Paths:  unfinished,
			Fix:    "say what problem the tool answers and why it works this way",
		})
	}

	repos := map[string]repoInfo{}
	ws := walkWorkspace(cfg, progress)
	report.Files = ws.files
	scripts := dedupeByCheckout(dropIgnored(cfg.Root, ws.scripts), repos)

	if len(ws.empties) > 0 {
		relative := make([]string, 0, len(ws.empties))
		quoted := make([]string, 0, len(ws.empties))
		for _, dir := range ws.empties {
			relative = append(relative, relTo(cfg.Root, dir))
			quoted = append(quoted, shellQuote(relTo(cfg.Root, dir)))
		}
		sort.Strings(relative)
		sort.Strings(quoted)
		add(finding{
			Kind:   "empty tool directories",
			Detail: fmt.Sprintf("%d", len(ws.empties)),
			Paths:  relative,
			Fix:    "cd " + shellQuote(cfg.Root) + " && rmdir " + strings.Join(quoted, " "),
		})
	}

	var orphans []string
	for _, path := range uncovered(result.Tools, scripts) {
		orphans = append(orphans, relTo(cfg.Root, path))
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		add(finding{
			Kind:   "script without manifest",
			Detail: fmt.Sprintf("%d", len(orphans)),
			Paths:  orphans,
			Fix:    "register with `jig add <path>`, or list it in " + ignoreFile,
		})
	}

	for _, group := range identicalGroups(scripts) {
		if sameFileAcrossCheckouts(group, repos) {
			continue
		}
		relative := make([]string, 0, len(group))
		for _, path := range group {
			relative = append(relative, relTo(cfg.Root, path))
		}
		add(finding{Kind: "identical files", Detail: filepath.Base(group[0]), Paths: relative})
	}

	if indexStale(cfg, result.Tools) {
		add(finding{
			Kind:   "TOOLS.md is stale",
			Detail: "it does not match the registry",
			Paths:  []string{relTo(cfg.Root, cfg.Index)},
			Fix:    "jig index",
		})
	}

	if checkouts := duplicateCheckouts(cfg.Root); len(checkouts) > 0 {
		remotes := make([]string, 0, len(checkouts))
		for remote := range checkouts {
			remotes = append(remotes, remote)
		}
		sort.Strings(remotes)

		lines := make([]string, 0, len(remotes))
		for _, remote := range remotes {
			lines = append(lines, shortRemote(remote)+": "+strings.Join(checkouts[remote], ", "))
		}
		add(finding{
			Kind:   "repository checked out more than once",
			Level:  levelInfo,
			Detail: fmt.Sprintf("%d — identical files across these are not duplicates", len(remotes)),
			Paths:  lines,
		})
	}

	return report
}

func writeDoctor(w io.Writer, report doctorReport, p palette) {
	_, _ = fmt.Fprintf(w, "%s%s%s · %d tools in the registry · %d files scanned\n\n",
		p.bold, displayRoot(report.Root), p.reset, report.Tools, report.Files)

	if report.problems() == 0 {
		_, _ = fmt.Fprintf(w, "%s✓ clean%s\n\n", p.green, p.reset)
	}

	for _, item := range report.Findings {
		tint := p.yellow
		if item.Level == levelInfo {
			tint = p.dim
		}
		_, _ = fmt.Fprintf(w, "%s%s%s  %s\n", tint, item.Kind, p.reset, item.Detail)

		shown := item.Paths
		if len(shown) > 12 {
			shown = groupByOwner(shown)
		}
		for _, path := range shown {
			_, _ = fmt.Fprintf(w, "   %s%s%s\n", p.dim, path, p.reset)
		}
		if item.Fix != "" {
			_, _ = fmt.Fprintf(w, "   %s→ %s%s\n", p.cyan, item.Fix, p.reset)
		}
		_, _ = fmt.Fprintln(w)
	}
}

func isScriptCandidate(cfg Config, path string) bool {
	name := filepath.Base(path)
	if strings.Contains(name, ".min.") || strings.HasSuffix(name, "_test.go") {
		return false
	}
	ext := filepath.Ext(name)
	if alwaysScript[ext] {
		return true
	}
	return toolDirExts[ext] && cfg.inToolDir(filepath.Dir(path))
}

func identicalGroups(paths []string) [][]string {
	byHash := map[string][]string{}
	shipped := sha256.Sum256([]byte(shippedScreen()))

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) == 0 {
			continue
		}
		sum := sha256.Sum256(raw)
		if sum == shipped {
			continue
		}
		key := hex.EncodeToString(sum[:])
		byHash[key] = append(byHash[key], path)
	}

	var groups [][]string
	for _, group := range byHash {
		if len(group) > 1 {
			sort.Strings(group)
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return groups
}

func duplicateCheckouts(base string) map[string][]string {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	cache := map[string]repoInfo{}
	byRemote := map[string][]string{}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		info := resolveRepo(dir, cache)
		if info.remote == "" || info.root != dir {
			continue
		}
		label := entry.Name()
		if info.branch != "" {
			label += " (" + info.branch + ")"
		}
		byRemote[info.remote] = append(byRemote[info.remote], label)
	}

	for remote, roots := range byRemote {
		if len(roots) < 2 {
			delete(byRemote, remote)
			continue
		}
		sort.Strings(roots)
	}
	return byRemote
}

func sameFileAcrossCheckouts(group []string, cache map[string]repoInfo) bool {
	key := ""
	for _, path := range group {
		info := resolveRepo(filepath.Dir(path), cache)
		if info.remote == "" {
			return false
		}
		rel, err := filepath.Rel(info.root, path)
		if err != nil {
			return false
		}
		current := info.remote + "\x00" + rel
		if key == "" {
			key = current
			continue
		}
		if key != current {
			return false
		}
	}
	return key != ""
}

func groupByOwner(paths []string) []string {
	counts := map[string]int{}
	for _, path := range paths {
		owner, _, found := strings.Cut(path, string(filepath.Separator))
		if !found {
			owner = "."
		}
		counts[owner]++
	}

	owners := make([]string, 0, len(counts))
	for owner := range counts {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		if counts[owners[i]] != counts[owners[j]] {
			return counts[owners[i]] > counts[owners[j]]
		}
		return owners[i] < owners[j]
	})

	out := make([]string, 0, len(owners))
	for _, owner := range owners {
		out = append(out, fmt.Sprintf("%-28s %d", owner, counts[owner]))
	}
	return out
}

func dropIgnored(base string, paths []string) []string {
	rules := map[string][]string{}
	kept := make([]string, 0, len(paths))
	for _, path := range paths {
		if !ignoredByRules(base, path, rules) {
			kept = append(kept, path)
		}
	}
	return kept
}

func ignoredByRules(base, path string, rules map[string][]string) bool {
	dir := filepath.Dir(path)
	for {
		patterns, cached := rules[dir]
		if !cached {
			patterns = readIgnore(filepath.Join(dir, ignoreFile))
			rules[dir] = patterns
		}
		if len(patterns) > 0 {
			if rel, err := filepath.Rel(dir, path); err == nil && matchesAny(patterns, rel) {
				return true
			}
		}
		if dir == base || dir == filepath.Dir(dir) {
			return false
		}
		dir = filepath.Dir(dir)
	}
}

func readIgnore(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var patterns []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, strings.TrimSuffix(line, "/"))
	}
	return patterns
}

func matchesAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, filepath.Base(rel)); ok {
			return true
		}
		if strings.HasPrefix(rel, pattern+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func dedupeByCheckout(paths []string, cache map[string]repoInfo) []string {
	best := map[string]string{}
	var order []string

	for _, path := range paths {
		info := resolveRepo(filepath.Dir(path), cache)
		key := path
		if info.remote != "" {
			if rel, err := filepath.Rel(info.root, path); err == nil {
				key = info.remote + "\x00" + rel
			}
		}
		current, seen := best[key]
		if !seen {
			best[key] = path
			order = append(order, key)
			continue
		}
		if preferPath(path, current, cache) {
			best[key] = path
		}
	}

	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, best[key])
	}
	return out
}

func preferPath(candidate, current string, cache map[string]repoInfo) bool {
	candidateTrunk := isTrunk(resolveRepo(filepath.Dir(candidate), cache).branch)
	currentTrunk := isTrunk(resolveRepo(filepath.Dir(current), cache).branch)
	if candidateTrunk != currentTrunk {
		return candidateTrunk
	}
	return len(candidate) < len(current)
}

func missingRunTarget(tool Tool) string {
	for field := range strings.FieldsSeq(tool.Run) {
		if !strings.HasPrefix(field, "./") && !strings.HasPrefix(field, "../") {
			continue
		}
		if _, err := os.Stat(filepath.Join(tool.WorkdirAbs, strings.TrimSuffix(field, "/..."))); err != nil {
			return field
		}
	}
	return ""
}

func placeholders(m Manifest) []string {
	var todo []string
	for _, field := range [][2]string{{"summary", m.Summary}, {"why", m.Why}} {
		if strings.HasPrefix(strings.TrimSpace(field[1]), "TODO") {
			todo = append(todo, field[0])
		}
	}
	return todo
}

func shortRemote(remote string) string {
	if _, path, found := strings.Cut(remote, ":"); found {
		return strings.TrimSuffix(path, ".git")
	}
	return remote
}

func newScreen(tool string) (*anim.Screen, error) {
	return anim.NewScreen(anim.Options{Mode: os.Getenv("JIG_ANIM"), Tool: tool})
}
