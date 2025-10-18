package daemon

import (
	"fmt"

	"github.com/alebeck/boring/internal/tunnel"
)

type CmdKind int

const (
	Nop CmdKind = iota
	Open
	Close
	List
	Shutdown
)

var cmdKindNames = map[CmdKind]string{
	Nop:      "Nop",
	Open:     "Open",
	Close:    "Close",
	List:     "List",
	Shutdown: "Shutdown",
}

func (k CmdKind) String() string {
	n, ok := cmdKindNames[k]
	if !ok {
		return fmt.Sprintf("%d", int(k))
	}
	return n
}

// Cmd represents a command sent to the daemon
type Cmd struct {
	Kind   CmdKind     `json:"kind"`
	Tunnel tunnel.Desc `json:"tunnel,omitempty"`
}
