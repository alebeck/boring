package daemon

import (
	"github.com/alebeck/boring/internal/tunnel"
)

// Info contains information about the daemon, e.g. the build commit
type Info struct {
	// Commit is the 5-character commit hash identifying the daemon build
	Commit string `json:"commit"`
}

// Resp represents a response from the daemon
type Resp struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Tunnels map[string]tunnel.Desc `json:"tunnels,omitempty"`
	Info    Info                   `json:"info,omitempty"`
}
