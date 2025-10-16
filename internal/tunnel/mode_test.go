package tunnel

import "testing"

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
