# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# Bash completion for the Jennifer interpreter CLI. Packaging installs
# this at /usr/share/bash-completion/completions/jennifer (and symlinks
# jennifer-tiny to it). For a local checkout, source it from your shell:
#
#   source packaging/completions/jennifer.bash
#
# It completes subcommands, then .j files for the file-taking ones, and
# the lint / profile flags and their values.

_jennifer() {
    local cur prev words cword
    if declare -F _init_completion >/dev/null 2>&1; then
        # -n = keeps `--format=value` a single word so we can split it.
        _init_completion -n = 2>/dev/null || return
    else
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        cword=$COMP_CWORD
        words=("${COMP_WORDS[@]}")
    fi

    local subcommands="run repl tokens ast fmt lint profile version help"

    # Find the subcommand: the first non-flag word after the program name.
    local sub="" i
    for (( i = 1; i < cword; i++ )); do
        case "${words[i]}" in
        -*) ;;
        *)
            sub="${words[i]}"
            break
            ;;
        esac
    done

    # No subcommand yet: complete the subcommand list.
    if [[ -z "$sub" ]]; then
        COMPREPLY=( $(compgen -W "$subcommands --help --version" -- "$cur") )
        return
    fi

    # Value completion for --format= / --checks= (works whether or not `=`
    # is a word break: match the joined `--flag=value` form, and fall back
    # to prev == "=" for the split form).
    local flagword="$cur"
    [[ "$prev" == "=" ]] && flagword="${words[cword-2]}=$cur"
    case "$flagword" in
    --format=*)
        local vals=""
        case "$sub" in
        lint) vals="human json github" ;;
        profile) vals="table pprof trace" ;;
        esac
        COMPREPLY=( $(compgen -W "$vals" -- "${flagword#--format=}") )
        return
        ;;
    --checks=*)
        # Comma-separated IDS, optionally negated with `!`; complete the
        # segment after the last comma.
        local ids="L001 L002 L003 L004 L005 L006 L007 L008 L009 L010"
        COMPREPLY=( $(compgen -W "$ids" -- "${flagword##*,}") )
        return
        ;;
    esac

    case "$sub" in
    run | tokens | ast | fmt)
        _jennifer_files "$cur"
        ;;
    lint)
        if [[ "$cur" == -* ]]; then
            COMPREPLY=( $(compgen -W "--checks= --format= --help" -- "$cur") )
            compopt -o nospace 2>/dev/null
        else
            _jennifer_files "$cur"
        fi
        ;;
    profile)
        if [[ "$cur" == -* ]]; then
            COMPREPLY=( $(compgen -W "--allocs --format= --help" -- "$cur") )
            [[ "$cur" == --format ]] && compopt -o nospace 2>/dev/null
        else
            _jennifer_files "$cur"
        fi
        ;;
    esac
}

# Complete .j source files (and directories to descend into). `-` for
# stdin is offered implicitly by the CLI, not the completion.
_jennifer_files() {
    local cur="$1"
    local IFS=$'\n'
    COMPREPLY=( $(compgen -f -X '!*.j' -- "$cur") $(compgen -d -S / -- "$cur") )
}

complete -F _jennifer jennifer
complete -F _jennifer jennifer-tiny
