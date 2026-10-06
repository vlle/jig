package e2e

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type pluginManifest struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Skills  []string `json:"skills"`
	Hooks   map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestPluginManifest(t *testing.T) {
	t.Parallel()
	var plugin pluginManifest
	readJSON(t, "../.claude-plugin/plugin.json", &plugin)

	var market struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	if len(market.Plugins) != 1 || market.Plugins[0].Name != plugin.Name || market.Plugins[0].Source != "./" {
		t.Fatalf("the marketplace must list %q from ./, got %+v", plugin.Name, market.Plugins)
	}

	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	latest := regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\]`).FindSubmatch(changelog)
	if latest == nil || string(latest[1]) != plugin.Version {
		t.Fatalf("plugin.json version %q must be the latest release in CHANGELOG.md", plugin.Version)
	}

	for _, dir := range plugin.Skills {
		skills, _ := filepath.Glob(filepath.Join("..", dir, "*", "SKILL.md"))
		if len(skills) == 0 {
			t.Fatalf("no skills in %s", dir)
		}
		for _, skill := range skills {
			raw, _ := os.ReadFile(skill)
			contains(t, skill, string(raw), "\nname: "+filepath.Base(filepath.Dir(skill))+"\n")
		}
	}

	for _, event := range []string{"SessionStart", "PreToolUse"} {
		if len(plugin.Hooks[event]) == 0 {
			t.Fatalf("no %s hook", event)
		}
		for _, group := range plugin.Hooks[event] {
			for _, hook := range group.Hooks {
				if hook.Command != `"${CLAUDE_PLUGIN_ROOT}"/bin/jig hook` {
					t.Fatalf("%s runs %q", event, hook.Command)
				}
			}
		}
	}

	if stat, err := os.Stat("../bin/jig"); err != nil || stat.Mode()&0o111 == 0 {
		t.Fatalf("bin/jig must exist and be executable: %v", err)
	}
}

func TestPluginLauncher(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("releases are built for darwin and linux on amd64 and arm64")
	}
	var plugin pluginManifest
	readJSON(t, "../.claude-plugin/plugin.json", &plugin)

	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	basePath := fakeBin + ":/usr/bin:/bin"

	launch := func(root string, env ...string) result {
		t.Helper()
		cmd := exec.Command(filepath.Join(root, "bin", "jig"), "version")
		cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=" + basePath}, env...)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		res := result{}
		var exitErr *exec.ExitError
		switch err := cmd.Run(); {
		case errors.As(err, &exitErr):
			res.code = exitErr.ExitCode()
		case err != nil:
			t.Fatal(err)
		}
		res.stdout, res.stderr = stdout.String(), stderr.String()
		return res
	}
	noRelease := "JIG_DOWNLOAD_URL=file:///nonexistent"

	releases := t.TempDir()
	publishRelease(t, filepath.Join(releases, "v"+plugin.Version), false)
	root := pluginCopy(t)
	r := launch(root, "JIG_DOWNLOAD_URL=file://"+releases)
	r.ok(t)
	contains(t, "stdout", r.stdout, "jig ")
	contains(t, "stderr", r.stderr, "downloading v"+plugin.Version)
	if _, err := os.Stat(filepath.Join(root, ".cache", "jig-v"+plugin.Version)); err != nil {
		t.Fatalf("the downloaded binary is not cached: %v", err)
	}

	r = launch(root, noRelease)
	r.ok(t)
	lacks(t, "stderr", r.stderr, "downloading")

	tampered := t.TempDir()
	publishRelease(t, filepath.Join(tampered, "v"+plugin.Version), true)
	root = pluginCopy(t)
	r = launch(root, "JIG_DOWNLOAD_URL=file://"+tampered)
	if r.code != 127 {
		t.Fatalf("a tampered archive must fail with 127, got %d\n%s", r.code, r.stderr)
	}
	contains(t, "stderr", r.stderr, "does not match checksums.txt")
	if left, _ := os.ReadDir(filepath.Join(root, ".cache")); len(left) != 0 {
		t.Fatalf("a rejected download left %v behind", left)
	}

	launch(pluginCopy(t), noRelease, "JIG_BIN="+jigBin).ok(t)

	other := t.TempDir()
	if err := os.Symlink(jigBin, filepath.Join(other, "jig")); err != nil {
		t.Fatal(err)
	}
	root = pluginCopy(t)
	launch(root, noRelease, "PATH="+filepath.Join(root, "bin")+":"+other+":"+basePath).ok(t)
}

func pluginCopy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"bin/jig", ".claude-plugin/plugin.json"} {
		raw, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), raw, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func publishRelease(t *testing.T, dir string, tamper bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(jigBin)
	if err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("jig_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	zipped := gzip.NewWriter(io.MultiWriter(file, hash))
	archive := tar.NewWriter(zipped)
	if err := archive.WriteHeader(&tar.Header{Name: "jig", Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(binary); err != nil {
		t.Fatal(err)
	}
	for _, closer := range []io.Closer{archive, zipped, file} {
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	if tamper {
		sum = strings.Repeat("0", len(sum))
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(sum+"  "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
