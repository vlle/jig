package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type runRecord struct {
	TS         string   `json:"ts"`
	Source     string   `json:"source"`
	RunID      string   `json:"run_id"`
	Cmd        string   `json:"cmd"`
	Argv       []string `json:"argv,omitempty"`
	Tool       string   `json:"tool,omitempty"`
	Safety     string   `json:"safety,omitempty"`
	Targets    []string `json:"targets,omitempty"`
	Run        string   `json:"run,omitempty"`
	Workdir    string   `json:"workdir,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	TTY        bool     `json:"tty"`
	Host       string   `json:"host,omitempty"`
	PID        int      `json:"pid"`
	Rev        string   `json:"jig_rev,omitempty"`
}

var secretFlagParts = []string{"token", "pass", "secret", "key", "cookie", "auth", "cred"}

func newRunRecord(started time.Time) runRecord {
	cwd, _ := os.Getwd()
	host, _ := os.Hostname()

	return runRecord{
		TS:     started.Format(time.RFC3339),
		Source: "jig",
		RunID:  strconv.FormatInt(started.UnixNano(), 36) + "-" + strconv.Itoa(os.Getpid()),
		Cwd:    cwd,
		TTY:    interactive(),
		Host:   host,
		PID:    os.Getpid(),
		Rev:    buildRevision(),
	}
}

func runlogPath() string {
	if custom := os.Getenv("JIG_LOG"); custom != "" {
		return custom
	}

	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "jig", "runs.jsonl")
}

func appendRunlog(record runRecord) {
	path := runlogPath()
	if path == "" || path == "-" {
		return
	}

	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	_, _ = file.Write(append(line, '\n'))
}

func isSecretFlag(name string) bool {
	if !strings.HasPrefix(name, "-") {
		return false
	}
	lowered := strings.ToLower(name)
	for _, part := range secretFlagParts {
		if strings.Contains(lowered, part) {
			return true
		}
	}
	return false
}

func redactArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	redacted := make([]string, 0, len(args))
	hideNext := false

	for _, arg := range args {
		if hideNext {
			redacted = append(redacted, "***")
			hideNext = false
			continue
		}

		name, _, inline := strings.Cut(arg, "=")
		if !isSecretFlag(name) {
			redacted = append(redacted, arg)
			continue
		}
		if inline {
			redacted = append(redacted, name+"=***")
			continue
		}
		redacted = append(redacted, arg)
		hideNext = true
	}

	return redacted
}
