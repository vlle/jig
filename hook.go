package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type hookInput struct {
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

type hookDecision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func cmdHook(in io.Reader, out io.Writer) error {
	if os.Getenv("JIG_HOOK") == "off" {
		return nil
	}

	var input hookInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return nil
	}

	reason := hookVerdict(input)
	if reason == "" {
		return nil
	}

	return json.NewEncoder(out).Encode(map[string]hookDecision{
		"hookSpecificOutput": {
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		},
	})
}

func hookVerdict(input hookInput) string {
	if input.ToolName != "Write" || input.ToolInput.FilePath == "" {
		return ""
	}

	path := input.ToolInput.FilePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(input.Cwd, path)
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		return ""
	}

	cfg, err := loadConfig(filepath.Dir(path))
	if err != nil {
		return ""
	}
	rel, inside := within(cfg.Root, path)
	if !inside {
		return ""
	}

	for dir := filepath.Dir(path); dir != cfg.Root && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if cfg.skipped(dir) || bundledDirs[filepath.Base(dir)] || dir == cfg.Registry {
			return ""
		}
	}
	if !isScriptCandidate(cfg, path) || len(dropIgnored(cfg.Root, []string{path})) == 0 {
		return ""
	}

	for _, tool := range scan(cfg).Tools {
		if path == tool.TargetAbs {
			return ""
		}
		if stat, statErr := os.Stat(tool.TargetAbs); statErr == nil && stat.IsDir() &&
			strings.HasPrefix(path, tool.TargetAbs+string(filepath.Separator)) {
			return ""
		}
	}

	query := searchWords(rel)
	return fmt.Sprintf("jig: %s would be a new script. Search the registry before writing one: `jig ls %s` (try a synonym too). "+
		"If a tool fits, reuse or extend it (`jig show <id>`, `jig src <id>`). "+
		"If nothing fits, scaffold with `jig new <id> --kind go|go-parallel|bash|node --dir %s`, then edit the generated files.",
		rel, query, shellQuote(filepath.Dir(path)))
}

func searchWords(rel string) string {
	stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if stem == "main" || stem == "run" || stem == "index" {
		stem = filepath.Base(filepath.Dir(rel))
	}
	words := strings.FieldsFunc(stem, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	if len(words) == 0 {
		return stem
	}
	return strings.Join(words, " ")
}
