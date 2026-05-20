package tunnel

// Forward is one port forwarding carried over a tunnel's SSH connection.
//
// A tunnel owns a single SSH connection and one or more forwards. Each forward
// has its own local/remote addresses and mode, mirroring the legacy
// single-forward shorthand fields on Desc.
//
// Mode is JSON-encoded as an integer over the IPC socket, exactly like
// Desc.Mode: it has a MarshalTOML method but is deliberately not a
// TextMarshaler. The json tag therefore omits "omitempty", since the zero
// value (Local) is a meaningful mode that must still cross the wire.
type Forward struct {
	Name          string      `toml:"name,omitempty" json:"name,omitempty"`
	LocalAddress  StringOrInt `toml:"local,omitempty" json:"local,omitempty"`
	RemoteAddress StringOrInt `toml:"remote,omitempty" json:"remote,omitempty"`
	Mode          Mode        `toml:"mode,omitempty" json:"mode"`
}

// Label returns a human-readable identifier for the forward, used in error
// messages, logs, and the `boring list` / TUI display: its configured Name,
// or its local address when unnamed.
func (f Forward) Label() string {
	if f.Name != "" {
		return f.Name
	}
	return string(f.LocalAddress)
}
