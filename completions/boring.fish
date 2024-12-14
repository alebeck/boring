function __boring_get_names
    set stat $argv[1]
    set used $argv[2..-1]
    set names

    # retrieve names based on status
    if test "$stat" = "closed"
        set names (boring list 2>/dev/null | awk 'NR > 1 && $1 == "closed" { print $2 }')
    else
        set names (boring list 2>/dev/null | awk 'NR > 1 && $1 != "closed" { print $2 }')
    end

    # filter names based on already provided arguments
    for name in $names
        set found 0
        for arg in $used
            if test "$name" = "$arg"
                set found 1
                break
            end
        end
        if test $found -eq 0
            echo $name
        end
    end
end

function __boring_complete
    set command (commandline -opc)[2]
    set arguments (commandline -opc)[3..-1]

    if test (count $command) -eq 0
        printf "%s\n" open close list edit version
        return
    end

    switch $command
        case open o
            __boring_get_names closed $arguments
        case close c
            __boring_get_names open $arguments
    end
end

complete -f -c boring -a "(__boring_complete)"
