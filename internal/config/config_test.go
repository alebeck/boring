package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	orig := Path
	t.Cleanup(func() { Path = orig })
	Path = filepath.Join(t.TempDir(), "missing.toml")
	if _, err := Load(); err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestSpecialPrefixEmpty(t *testing.T) {
	if specialPrefix("") {
		t.Error(`specialPrefix("") = true, want false`)
	}
}
