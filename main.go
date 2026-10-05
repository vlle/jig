package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

const usage = `jig — find, run and scaffold the scripts in your workspace

  jig                                         home screen and tool browser
  jig ls     [QUERY] [--search Q] [--tag T] [--kind K] [--all] [--json|--ids]
  jig show   <id> [--json]                    manifest, flags, safety, where it runs
  jig src    <id>                             source of the tool
  jig run    [--yes] <id> [-- args...]        run from the tool's workdir
  jig new    <id> [--kind KIND] [--dir DIR] [--force]
                                              scaffold: go, go-parallel, bash, node
  jig add    <path>... [--id ID] [--summary S] [--why W] [--safety S]
             [--targets a,b] [--tags a,b] [--run CMD]
                                              register scripts that already exist
  jig doctor [--json]                         orphans, broken manifests, stale index;
                                              exits 1 when it finds a problem
  jig index  [--out PATH]                     regenerate TOOLS.md
  jig init   [DIR]                            create jig.yml and an empty registry
  jig demo   [--only NAME] [--speed N]        the animation kit scaffolds ship with
  jig agent  rules|skill                      text to install jig into a coding agent
  jig hook                                    Claude Code PreToolUse hook (stdin JSON)
  jig completion bash|zsh|fish                shell completion, tool ids included
  jig version                                 version, commit, Go and platform

Search before you write: jig ls <words>. Reuse or extend; scaffold only when nothing fits.
Workspace: the nearest jig.yml above the current directory, or $JIG_ROOT.
`

func main() {
	started := time.Now()
	record := newRunRecord(started)

	code := dispatch(os.Args[1:], &record)

	record.ExitCode = code
	record.DurationMS = time.Since(started).Milliseconds()
	appendRunlog(record)

	if code != 0 {
		os.Exit(code)
	}
}

func dispatch(argv []string, record *runRecord) int {
	if wantsHelp(argv) {
		fmt.Print(usage)
		return 0
	}

	command, args := "home", []string(nil)
	if len(argv) > 0 {
		command, args = argv[0], argv[1:]
	}
	record.Cmd = command
	record.Argv = redactArgs(args)

	switch command {
	case "version", "--version":
		return exitCode(cmdVersion())
	case "init":
		return exitCode(cmdInit(args))
	case "demo":
		return exitCode(cmdDemo(args))
	case "agent":
		return exitCode(cmdAgent(args))
	case "completion":
		return exitCode(cmdCompletion(args))
	case "hook":
		return exitCode(cmdHook(os.Stdin, os.Stdout))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return exitCode(err)
	}
	cfg, err := loadConfig(cwd)
	if err != nil {
		return exitCode(err)
	}

	switch command {
	case "home":
		err = runTUI(cfg, record)
	case "ls", "list":
		err = cmdList(cfg, args)
	case "show":
		err = cmdShow(cfg, args)
	case "src", "source":
		err = cmdSource(cfg, args)
	case "run":
		err = cmdRun(cfg, args, record)
	case "doctor":
		err = cmdDoctor(cfg, args)
	case "new":
		err = cmdNew(cfg, args)
	case "add":
		err = cmdAdd(cfg, args)
	case "index":
		err = cmdIndex(cfg, args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", command, usage)
		return 2
	}

	return exitCode(err)
}

func wantsHelp(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	if slices.Contains([]string{"help", "-h", "--help"}, argv[0]) {
		return true
	}
	return len(argv) > 1 && (argv[1] == "-h" || argv[1] == "--help")
}

type exitError struct {
	code int
}

func (e exitError) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}

	var exit exitError
	if errors.As(err, &exit) {
		return exit.code
	}

	fmt.Fprintf(os.Stderr, "%serror:%s %v\n", color.red, color.reset, err)
	return 1
}

func flagValue(args []string, name string) (string, []string) {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1], slices.Concat(args[:i], args[i+2:])
		}
		if value, found := strings.CutPrefix(arg, name+"="); found {
			return value, slices.Concat(args[:i], args[i+1:])
		}
	}
	return "", args
}

func hasFlag(args []string, name string) (bool, []string) {
	for i, arg := range args {
		if arg == name {
			return true, slices.Concat(args[:i], args[i+1:])
		}
	}
	return false, args
}

func splitPassthrough(args []string) (own, passthrough []string) {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i], args[i+1:]
	}
	return args, nil
}

func emitJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printManifestErrors(errs []string) {
	for _, message := range errs {
		fmt.Fprintf(os.Stderr, "%smanifest:%s %s\n", color.yellow, color.reset, message)
	}
}
