package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Arg struct {
	Flag    string `yaml:"flag" json:"flag"`
	Desc    string `yaml:"desc" json:"desc"`
	Example string `yaml:"example,omitempty" json:"example,omitempty"`
}

type Manifest struct {
	ID      string   `yaml:"id" json:"id"`
	Kind    string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Path    string   `yaml:"path" json:"path"`
	Summary string   `yaml:"summary" json:"summary"`
	Run     string   `yaml:"run" json:"run"`
	Workdir string   `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	Safety  string   `yaml:"safety" json:"safety"`
	Targets []string `yaml:"targets,omitempty" json:"targets,omitempty"`
	Guard   string   `yaml:"guard,omitempty" json:"guard,omitempty"`
	Env     string   `yaml:"env,omitempty" json:"env,omitempty"`
	Args    []Arg    `yaml:"args,omitempty" json:"args,omitempty"`
	Why     string   `yaml:"why,omitempty" json:"why,omitempty"`
	Tags    []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Status  string   `yaml:"status,omitempty" json:"status,omitempty"`
	Ticket  string   `yaml:"ticket,omitempty" json:"ticket,omitempty"`
	Source  string   `yaml:"source,omitempty" json:"source,omitempty"`
}

type Tool struct {
	Manifest
	ManifestPath string `json:"manifest_path"`
	RepoRoot     string `json:"repo_root"`
	RepoRemote   string `json:"repo_remote"`
	RepoRel      string `json:"repo_rel"`
	Branch       string `json:"branch"`
	WorkdirAbs   string `json:"workdir_abs"`
	TargetAbs    string `json:"target_abs"`
	TargetDir    string `json:"target_dir"`
	SourceAbs    string `json:"source_abs,omitempty"`
	NeedsConfirm bool   `json:"needs_confirm"`
	Registered   bool   `json:"registered"`
	Origin       string `json:"origin"`
}

var (
	validSafety = map[string]bool{"read-only": true, "writes": true, "destructive": true}
	validStatus = map[string]bool{"active": true, "incident-only": true, "deprecated": true}
	validKind   = map[string]bool{"tool": true, "env": true, "suite": true}
	validGuard  = map[string]bool{"": true, "self": true}
)

func loadManifest(path string) (Manifest, error) {
	var m Manifest

	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}

	if m.Status == "" {
		m.Status = "active"
	}
	if m.Kind == "" {
		m.Kind = "tool"
	}
	if m.Safety == "" {
		m.Safety = "read-only"
	}

	return m, m.validate(path)
}

func (m Manifest) validate(path string) error {
	var missing []string
	for _, field := range [][2]string{{"id", m.ID}, {"summary", m.Summary}, {"run", m.Run}, {"path", m.Path}} {
		if field[1] == "" {
			missing = append(missing, field[0])
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing required fields: %s", path, strings.Join(missing, ", "))
	}
	if !validSafety[m.Safety] {
		return fmt.Errorf("%s: safety=%q, expected read-only|writes|destructive", path, m.Safety)
	}
	if !validStatus[m.Status] {
		return fmt.Errorf("%s: status=%q, expected active|incident-only|deprecated", path, m.Status)
	}
	if !validKind[m.Kind] {
		return fmt.Errorf("%s: kind=%q, expected tool|env|suite", path, m.Kind)
	}
	if !validGuard[m.Guard] {
		return fmt.Errorf("%s: guard=%q, expected self or nothing", path, m.Guard)
	}
	return nil
}

func (t Tool) guardsItself() bool {
	return t.Guard == "self"
}

func (t Tool) matches(query string) bool {
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		t.ID, t.Summary, t.Why, strings.Join(t.Tags, " "), t.Path,
	}, " "))
	for term := range strings.FieldsSeq(strings.ToLower(query)) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func (t Tool) hasTag(tag string) bool {
	for _, candidate := range t.Tags {
		if strings.EqualFold(candidate, tag) {
			return true
		}
	}
	return false
}

var flowSafe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@+-]*$`)

func manifestYAML(m Manifest) string {
	var out strings.Builder
	field := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&out, "%s: %s\n", key, yamlScalar(value))
		}
	}

	field("id", m.ID)
	if m.Kind != "tool" {
		field("kind", m.Kind)
	}
	field("path", m.Path)
	field("summary", m.Summary)
	field("run", m.Run)
	field("workdir", m.Workdir)
	fmt.Fprintf(&out, "safety: %s  # read-only | writes | destructive\n", yamlScalar(m.Safety))
	fmt.Fprintf(&out, "targets: %s\n", yamlList(m.Targets))
	field("guard", m.Guard)
	field("env", m.Env)
	if why := strings.TrimSpace(m.Why); why != "" {
		out.WriteString("why: |\n")
		for line := range strings.SplitSeq(why, "\n") {
			if line = strings.TrimRight(line, " \t\r"); line == "" {
				out.WriteString("\n")
				continue
			}
			out.WriteString("  " + line + "\n")
		}
	}
	fmt.Fprintf(&out, "tags: %s\n", yamlList(m.Tags))
	field("status", m.Status)
	field("ticket", m.Ticket)
	return out.String()
}

func yamlScalar(value string) string {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return strings.TrimSuffix(string(raw), "\n")
}

func yamlList(values []string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		var parsed any
		if flowSafe.MatchString(value) && yaml.Unmarshal([]byte(value), &parsed) == nil && parsed == value {
			items = append(items, value)
			continue
		}
		items = append(items, strconv.Quote(value))
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func splitList(value string) []string {
	var items []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}
