package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type runner struct {
	name    string
	summary string
	run     string
}

var runnerFiles = map[string]string{
	"Makefile": "make", "makefile": "make", "GNUmakefile": "make",
	"justfile": "just", "Justfile": "just", ".justfile": "just",
	"package.json":  "npm",
	"Taskfile.yml":  "task",
	"Taskfile.yaml": "task",
	"taskfile.yml":  "task",
	"taskfile.yaml": "task",
}

var (
	makeName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_+-]*$`)
	makeDirective = map[string]bool{
		"export": true, "unexport": true, "override": true, "private": true, "include": true, "-include": true,
		"sinclude": true, "vpath": true, "ifeq": true, "ifneq": true, "ifdef": true, "ifndef": true,
		"else": true, "endif": true, "define": true, "endef": true, "undefine": true,
	}
	justKeyword  = map[string]bool{"set": true, "alias": true, "export": true, "import": true, "mod": true, "unexport": true}
	justDoc      = regexp.MustCompile(`doc\(\s*(?:"([^"]*)"|'([^']*)')\s*\)`)
	npmLifecycle = map[string]bool{
		"preinstall": true, "install": true, "postinstall": true, "prepublish": true, "preprepare": true,
		"prepare": true, "postprepare": true, "prepublishOnly": true, "prepack": true, "postpack": true,
		"publish": true, "postpublish": true, "preversion": true, "version": true, "postversion": true,
		"dependencies": true,
	}
	npmLockfiles = [][2]string{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"}, {"bun.lock", "bun"}, {"package-lock.json", "npm"}}
)

func readRunners(path, root string) []runner {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	switch runnerFiles[filepath.Base(path)] {
	case "make":
		return makeTargets(string(raw))
	case "just":
		return justRecipes(string(raw))
	case "npm":
		return npmScripts(raw, packageManager(filepath.Dir(path), root))
	case "task":
		return taskfileTasks(raw)
	}
	return nil
}

func makeTargets(text string) []runner {
	var out []runner
	var comments []string
	seen := map[string]bool{}
	inDefine := false

	for raw := range strings.SplitSeq(text, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case inDefine:
			inDefine = trimmed != "endef"
			continue
		case strings.HasPrefix(trimmed, "define ") || strings.HasPrefix(trimmed, "override define "):
			inDefine, comments = true, nil
			continue
		case strings.HasPrefix(line, "\t"), trimmed == "":
			comments = nil
			continue
		case strings.HasPrefix(trimmed, "#"):
			comments = append(comments, strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
			continue
		}

		names, described := makeRule(line)
		above := comments
		comments = nil
		summary := firstNonEmpty(described, strings.Join(above, " "))
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				out = append(out, runner{name: name, summary: sentence(dropLeadingName(summary, name)), run: "make " + name})
			}
		}
	}
	return out
}

func makeRule(line string) (names []string, summary string) {
	if before, after, found := strings.Cut(line, "##"); found {
		line, summary = before, strings.TrimSpace(after)
	} else if before, _, found := strings.Cut(line, "#"); found {
		line = before
	}

	head, rest, found := strings.Cut(line, ":")
	if !found || strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ":=") {
		return nil, ""
	}
	if strings.ContainsAny(head, "=$%()\\/") {
		return nil, ""
	}
	fields := strings.Fields(head)
	if len(fields) == 0 || makeDirective[fields[0]] {
		return nil, ""
	}
	for _, name := range fields {
		if makeName.MatchString(name) {
			names = append(names, name)
		}
	}
	return names, summary
}

func justRecipes(text string) []runner {
	var out []runner
	var comments []string
	doc, private := "", false
	reset := func() { comments, doc, private = nil, "", false }

	for raw := range strings.SplitSeq(text, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		switch {
		case line == "", strings.HasPrefix(line, " "), strings.HasPrefix(line, "\t"):
			reset()
			continue
		case strings.HasPrefix(line, "#!"):
			continue
		case strings.HasPrefix(line, "#"):
			comments = append(comments, strings.TrimSpace(strings.TrimLeft(line, "#")))
			continue
		case strings.HasPrefix(line, "["):
			if strings.Contains(line, "private") {
				private = true
			}
			if match := justDoc.FindStringSubmatch(line); match != nil {
				doc = firstNonEmpty(match[1], match[2])
			}
			continue
		}

		name, ok := justHeader(line)
		summary := firstNonEmpty(doc, strings.Join(comments, " "))
		skip := !ok || private || strings.HasPrefix(name, "_")
		reset()
		if !skip {
			out = append(out, runner{name: name, summary: sentence(dropLeadingName(summary, name)), run: "just " + name})
		}
	}
	return out
}

func justHeader(line string) (string, bool) {
	line = strings.TrimPrefix(line, "@")
	end := 0
	for end < len(line) && isNameByte(line[end]) {
		end++
	}
	if end == 0 || justKeyword[line[:end]] {
		return "", false
	}

	quote := byte(0)
	for i := end; i < len(line); i++ {
		switch c := line[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ':':
			if i+1 < len(line) && line[i+1] == '=' {
				return "", false
			}
			return line[:end], true
		case c == '=' && i+1 < len(line) && line[i+1] == '=':
			return "", false
		}
	}
	return "", false
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func npmScripts(raw []byte, manager string) []runner {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return nil
	}

	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		if !npmLifecycle[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make([]runner, 0, len(names))
	for _, name := range names {
		command := strings.Join(strings.Fields(pkg.Scripts[name]), " ")
		out = append(out, runner{name: name, summary: clip(command, summaryLength), run: manager + " run " + shellQuote(name)})
	}
	return out
}

func packageManager(dir, root string) string {
	for {
		for _, lock := range npmLockfiles {
			if fileExists(filepath.Join(dir, lock[0])) {
				return lock[1]
			}
		}
		if dir == root || dir == filepath.Dir(dir) {
			return "npm"
		}
		dir = filepath.Dir(dir)
	}
}

func taskfileTasks(raw []byte) []runner {
	var file struct {
		Tasks yaml.Node `yaml:"tasks"`
	}
	if yaml.Unmarshal(raw, &file) != nil || file.Tasks.Kind != yaml.MappingNode {
		return nil
	}

	var out []runner
	for i := 0; i+1 < len(file.Tasks.Content); i += 2 {
		name := file.Tasks.Content[i].Value
		var task struct {
			Desc     string `yaml:"desc"`
			Summary  string `yaml:"summary"`
			Internal bool   `yaml:"internal"`
		}
		if body := file.Tasks.Content[i+1]; body.Kind == yaml.MappingNode {
			_ = body.Decode(&task)
		}
		if task.Internal || name == "" {
			continue
		}
		summary, _, _ := strings.Cut(strings.TrimSpace(task.Summary), "\n")
		out = append(out, runner{name: name, summary: sentence(firstNonEmpty(task.Desc, summary)), run: "task " + shellQuote(name)})
	}
	return out
}
