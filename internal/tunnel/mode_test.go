package tunnel

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestModeUnmarshalInvalid(t *testing.T) {
	var m Mode
	data := "invalid"
	if err := m.UnmarshalTOML(data); err == nil || err.Error() != "invalid mode" {
		t.Errorf("incorrect error: %v", err)
	}
}

func TestModeUnmarshalInvalidType(t *testing.T) {
	var m Mode
	data := 1
	if err := m.UnmarshalTOML(data); err == nil || err.Error() != "invalid mode type" {
		t.Errorf("incorrect error: %v", err)
	}
}

func TestModeConfigValueRoundTrip(t *testing.T) {
	for _, name := range []string{"local", "remote", "socks", "socks-remote"} {
		var m Mode
		if err := m.UnmarshalTOML(name); err != nil {
			t.Fatalf("UnmarshalTOML(%q): %v", name, err)
		}
		if m.ConfigValue() != name {
			t.Fatalf("round trip: got %q, want %q", m.ConfigValue(), name)
		}
	}
}

func TestModeEncodesAsTOMLString(t *testing.T) {
	var buf bytes.Buffer
	type holder struct {
		Mode Mode `toml:"mode"`
	}
	if err := toml.NewEncoder(&buf).Encode(holder{Mode: Remote}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, `mode = "remote"`) {
		t.Fatalf("Mode did not encode as a quoted TOML string; got: %q", got)
	}
}

func TestModeJSONStaysInteger(t *testing.T) {
	// Mode MUST keep encoding as an integer over JSON — that is the IPC wire
	// format between the CLI and daemon. Only TOML encoding uses the string
	// form. This test locks that the TOML marshaler did not leak into JSON.
	b, err := json.Marshal(Remote)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1" {
		t.Fatalf("Mode JSON form changed to %q; this would break the IPC protocol", b)
	}
	var m Mode
	if err := json.Unmarshal([]byte("1"), &m); err != nil {
		t.Fatalf("JSON int decode: %v", err)
	}
	if m != Remote {
		t.Fatalf("JSON int decode: got %v, want Remote", m)
	}
}
