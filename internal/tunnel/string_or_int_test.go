package tunnel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestStringOrIntInvalidType(t *testing.T) {
	var s StringOrInt
	if err := s.UnmarshalTOML(struct{}{}); err == nil ||
		!strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("incorrect error: %v", err)
	}
}

func TestStringOrIntEncodesAsTOMLString(t *testing.T) {
	var buf bytes.Buffer
	type holder struct {
		Local StringOrInt `toml:"local"`
	}
	if err := toml.NewEncoder(&buf).Encode(holder{Local: "9000"}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, `local = "9000"`) {
		t.Fatalf("StringOrInt did not encode as a quoted TOML string; got: %q", got)
	}
}

func TestStringOrIntFromIntEncodesAsString(t *testing.T) {
	// A bare integer in the config (local = 9000) is decoded to "9000" by
	// UnmarshalTOML; it must then re-encode as the quoted string "9000".
	var s StringOrInt
	if err := s.UnmarshalTOML(int64(9000)); err != nil {
		t.Fatalf("UnmarshalTOML(int64): %v", err)
	}
	var buf bytes.Buffer
	type holder struct {
		Local StringOrInt `toml:"local"`
	}
	if err := toml.NewEncoder(&buf).Encode(holder{Local: s}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, `local = "9000"`) {
		t.Fatalf("int-origin StringOrInt did not encode as a quoted string; got: %q", got)
	}
}
