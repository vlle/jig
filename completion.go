package main

import (
	"errors"
	"fmt"
)

const bashCompletion = `_jig() {
  local cur=${COMP_WORDS[COMP_CWORD]} prev=${COMP_WORDS[COMP_CWORD-1]}
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=($(compgen -W "ls show src run new add doctor index init demo agent hook version completion help" -- "$cur"))
    return
  fi
  case ${COMP_WORDS[1]} in
    show|src|run) COMPREPLY=($(compgen -W "$(JIG_LOG=- jig ls --all --ids --registered 2>/dev/null)" -- "$cur")) ;;
    new) [[ $prev == --kind ]] && COMPREPLY=($(compgen -W "go go-parallel bash node" -- "$cur")) ;;
    add) [[ $prev == --safety ]] && COMPREPLY=($(compgen -W "read-only writes destructive" -- "$cur")) ;;
    agent) COMPREPLY=($(compgen -W "rules skill" -- "$cur")) ;;
    completion) COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")) ;;
  esac
}
complete -o default -F _jig jig
`

const zshCompletion = `autoload -U +X bashcompinit && bashcompinit
` + bashCompletion

const fishCompletion = `complete -c jig -f
complete -c jig -n __fish_use_subcommand -a "ls show src run new add doctor index init demo agent hook version completion help"
complete -c jig -n "__fish_seen_subcommand_from show src run" -a "(env JIG_LOG=- jig ls --all --ids --registered 2>/dev/null)"
complete -c jig -n "__fish_seen_subcommand_from new" -l kind -xa "go go-parallel bash node"
complete -c jig -n "__fish_seen_subcommand_from new" -l dir -r -F
complete -c jig -n "__fish_seen_subcommand_from add" -F
complete -c jig -n "__fish_seen_subcommand_from add" -l safety -xa "read-only writes destructive"
complete -c jig -n "__fish_seen_subcommand_from agent" -a "rules skill"
complete -c jig -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"
`

var completions = map[string]string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion}

func cmdCompletion(args []string) error {
	if len(args) != 1 || completions[args[0]] == "" {
		return errors.New("usage: jig completion bash|zsh|fish")
	}
	fmt.Print(completions[args[0]])
	return nil
}
