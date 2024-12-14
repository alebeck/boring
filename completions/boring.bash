_boring() {
    local cur cmd
    cur="${COMP_WORDS[COMP_CWORD]}"

    local commands=("open" "close" "list" "edit" "version")

    _boring_get_names() {
        local status="$1"
        local -a names

        # retrieve tunnel names based on command
        if [[ "$status" == "closed" ]]; then
            names=($(boring list 2>/dev/null | awk 'NR > 1 && $1 == "closed" { print $2 }'))
        else
            names=($(boring list 2>/dev/null | awk 'NR > 1 && $1 != "closed" { print $2 }'))
        fi

        # filter names based on already provided arguments
        result=()
        for name in "${names[@]}"; do
            found=0
            for arg in "${COMP_WORDS[@]:1}"; do
                if [[ "$name" == "$arg" ]]; then
                    found=1
                    break
                fi
            done
            if [[ $found -eq 0 ]]; then
                result+=("$name")
            fi
        done

        COMPREPLY=($(compgen -W "${result[*]}" -- "$cur"))
    }

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands[*]}" -- "$cur"))
    elif [[ $COMP_CWORD -ge 2 ]]; then
        cmd="${COMP_WORDS[1]}"
        if [[ "$cmd" == "open" || "$cmd" == "o" ]]; then
            _boring_get_names "closed"
        elif [[ "$cmd" == "close" || "$cmd" == "c" ]]; then
            _boring_get_names "open"
        fi
    fi
}

complete -F _boring boring
