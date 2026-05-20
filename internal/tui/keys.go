package tui

// Dashboard key bindings, matched against tea.KeyMsg.String().
const (
	keyQuit    = "q"
	keyCtrlC   = "ctrl+c"
	keyUp      = "up"
	keyDown    = "down"
	keyVimUp   = "k"
	keyVimDown = "j"
	keyHelp    = "?"
	keyEnter   = "enter"
	keySpace   = " "
	keyAdd     = "a"
	keyEdit    = "e"
)

// Form key bindings, matched against tea.KeyMsg.String().
const (
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyLeft     = "left"
	keyRight    = "right"
	keyEsc      = "esc"
)
