package tunnel

import (
	"errors"
	"strconv"
	"strings"
)

type Mode int

const (
	Local Mode = iota
	Remote
	Socks
	RemoteSocks
)

func (m *Mode) UnmarshalTOML(data any) error {
	s, ok := data.(string)
	if !ok {
		return errors.New("invalid mode type")
	}

	switch strings.ToLower(s) {
	case "local", "l", "-l":
		*m = Local
	case "remote", "r", "-r":
		*m = Remote
	case "socks":
		*m = Socks
	case "socks-remote":
		*m = RemoteSocks
	default:
		return errors.New("invalid mode")
	}

	return nil
}

func (m Mode) String() string {
	if m == Local || m == Socks {
		return "->"
	}
	return "<-"
}

// ConfigValue returns the canonical TOML representation of the mode — the same
// strings UnmarshalTOML accepts. (String() returns a display arrow instead and
// must not be used for encoding or error messages.)
func (m Mode) ConfigValue() string {
	switch m {
	case Remote:
		return "remote"
	case Socks:
		return "socks"
	case RemoteSocks:
		return "socks-remote"
	default:
		return "local"
	}
}

// MarshalTOML implements the github.com/BurntSushi/toml Marshaler interface so
// the TOML encoder writes the mode as its canonical quoted string. It is
// deliberately NOT encoding.TextMarshaler: Desc.Mode is also JSON-encoded over
// the IPC socket, and a TextMarshaler would change that wire format too.
func (m Mode) MarshalTOML() ([]byte, error) {
	return []byte(strconv.Quote(m.ConfigValue())), nil
}
