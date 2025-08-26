package tunnel

import (
	"strings"
	"testing"
)

func TestStringOrIntInvalidType(t *testing.T) {
	var s StringOrInt
	if err := s.UnmarshalTOML(struct{}{}); err == nil ||
		!strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("incorrect error: %v", err)
	}
}
