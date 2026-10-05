package main

import (
	"embed"
	"errors"
	"os"
)

//go:embed agent
var agentFiles embed.FS

var agentTexts = map[string]string{
	"rules": "agent/RULES.md",
	"skill": "agent/skills/jig/SKILL.md",
}

func cmdAgent(args []string) error {
	if len(args) != 1 || agentTexts[args[0]] == "" {
		return errors.New("usage: jig agent rules|skill")
	}
	raw, err := agentFiles.ReadFile(agentTexts[args[0]])
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(raw)
	return err
}
