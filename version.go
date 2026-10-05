package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

var version string

func cmdVersion() error {
	fmt.Println(versionLine())
	return nil
}

func versionLine() string {
	release := releaseVersion()
	details := []string{runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH}
	if revision := buildRevision(); revision != "" && !strings.Contains(release, strings.TrimSuffix(revision, "+dirty")) {
		details = append([]string{revision}, details...)
	}
	return fmt.Sprintf("jig %s (%s)", release, strings.Join(details, ", "))
}

func releaseVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	revision, dirty := "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
			if len(revision) > 12 {
				revision = revision[:12]
			}
		case "vcs.modified":
			if setting.Value == "true" {
				dirty = "+dirty"
			}
		}
	}
	return revision + dirty
}
