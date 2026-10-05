package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const configName = "jig.yml"

type Config struct {
	Root        string   `yaml:"-" json:"root"`
	File        string   `yaml:"-" json:"file,omitempty"`
	Registry    string   `yaml:"registry" json:"registry"`
	Index       string   `yaml:"index" json:"index"`
	Skip        []string `yaml:"skip" json:"skip"`
	ToolDirs    []string `yaml:"tool_dirs" json:"tool_dirs"`
	ProdTargets []string `yaml:"prod_targets" json:"prod_targets"`
	Envs        []string `yaml:"envs" json:"envs"`
}

var builtinSkip = []string{"vendor", "node_modules", "dist", "build", "target", "__pycache__"}

func defaultConfig() Config {
	return Config{
		Registry:    filepath.Join(".jig", "registry"),
		Index:       "TOOLS.md",
		ToolDirs:    []string{"scripts", "tools"},
		ProdTargets: []string{"prod", "prd", "production"},
	}
}

var errNoRoot = errors.New("no " + configName + " here or in any parent directory; run `jig init` in your workspace or set JIG_ROOT")

func loadConfig(start string) (Config, error) {
	cfg := defaultConfig()

	root := os.Getenv("JIG_ROOT")
	switch {
	case root != "":
		if candidate := filepath.Join(root, configName); fileExists(candidate) {
			cfg.File = candidate
		}
	default:
		cfg.File = findConfig(start)
		if cfg.File == "" {
			return cfg, errNoRoot
		}
		root = filepath.Dir(cfg.File)
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return cfg, fmt.Errorf("resolve root %s: %w", root, err)
	}
	cfg.Root = abs

	if cfg.File != "" {
		if err := cfg.read(cfg.File); err != nil {
			return cfg, err
		}
	}

	if custom := os.Getenv("JIG_REGISTRY"); custom != "" {
		cfg.Registry = custom
	}
	if custom := os.Getenv("JIG_INDEX"); custom != "" {
		cfg.Index = custom
	}
	cfg.Registry = cfg.resolve(cfg.Registry)
	if cfg.Index != "" && cfg.Index != "-" {
		cfg.Index = cfg.resolve(cfg.Index)
	}
	return cfg, nil
}

func (c *Config) read(file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(c); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

func findConfig(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if candidate := filepath.Join(dir, configName); fileExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (c Config) resolve(value string) string {
	if strings.HasPrefix(value, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, value[2:])
		}
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(c.Root, value)
}

func (c Config) skipped(dir string) bool {
	if dir == c.Root {
		return false
	}
	name := filepath.Base(dir)
	if strings.HasPrefix(name, ".") {
		return true
	}

	rel := filepath.ToSlash(relTo(c.Root, dir))
	for _, pattern := range slices.Concat(builtinSkip, c.Skip) {
		target := name
		if anchored, found := strings.CutPrefix(pattern, "/"); found {
			pattern, target = anchored, rel
		} else if strings.Contains(pattern, "/") {
			target = rel
		}
		if ok, _ := path.Match(pattern, target); ok {
			return true
		}
	}
	return false
}

func (c Config) prod(targets []string) bool {
	for _, target := range targets {
		if slices.Contains(c.ProdTargets, target) {
			return true
		}
	}
	return false
}

func (c Config) inToolDir(dir string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(relTo(c.Root, dir)), "/") {
		if slices.Contains(c.ToolDirs, part) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && !stat.IsDir()
}

func relTo(base, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
}

const starterConfig = `# jig workspace config. Every key is optional; the values below are the defaults.

# where manifests live, relative to this file
registry: .jig/registry

# generated catalogue of tools; "-" disables it
index: TOOLS.md

# directories doctor never walks, on top of vendor, node_modules, dist, build, target,
# __pycache__ and hidden ones. A bare name matches at any depth, "a/b" matches a path
# relative to this file, a leading "/" anchors a name to this directory.
skip: []

# any .go/.js/.mjs/.py file below these directories counts as a script; .sh always does
tool_dirs: [scripts, tools]

# targets that make writes|destructive tools require --yes
prod_targets: [prod, prd, production]

# environments the scaffolds accept in -env; first one is the default, empty drops the flag
envs: []
`
