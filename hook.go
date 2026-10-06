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
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	Cwd           string `json:"cwd"`
	ToolInput     struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

type hookOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

func cmdHook(in io.Reader, out io.Writer) error {
	if os.Getenv("JIG_HOOK") == "off" {
		return nil
	}

	var input hookInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return nil
	}

	output, ok := hookResponse(input)
	if !ok {
		return nil
	}
	return json.NewEncoder(out).Encode(map[string]hookOutput{"hookSpecificOutput": output})
}

func hookResponse(input hookInput) (hookOutput, bool) {
	if input.HookEventName == "SessionStart" {
		text := sessionContext(input.Cwd)
		return hookOutput{HookEventName: "SessionStart", AdditionalContext: text}, text != ""
	}
	reason := hookVerdict(input)
	return hookOutput{HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: reason}, reason != ""
}

func sessionContext(cwd string) string {
	cfg, err := loadConfig(cwd)
	if err != nil {
		return ""
	}
	rules, err := agentFiles.ReadFile(agentTexts["rules"])
	if err != nil {
		return ""
	}

	tools := 0
	for _, tool := range scan(cfg).Tools {
		if tool.Kind != "env" && tool.Status != "deprecated" {
			tools++
		}
	}
	return fmt.Sprintf("This session runs in the jig workspace %s (%s registered).\n\n%s", cfg.Root, plural(tools, "tool"), rules)
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
