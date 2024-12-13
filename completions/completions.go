package completions

import _ "embed"

//go:embed boring.bash
var Bash string

//go:embed boring.zsh
var Zsh string

//go:embed boring.fish
var Fish string
