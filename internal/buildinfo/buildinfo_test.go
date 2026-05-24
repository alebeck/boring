package buildinfo

import "testing"

func TestLoadEnvOverrides(t *testing.T) {
	origCommit, origVersion := Commit, Version
	t.Cleanup(func() {
		Commit, Version = origCommit, origVersion
	})

	t.Setenv("BORING_COMMIT_OVERRIDE", "abcde")
	t.Setenv("BORING_VERSION_OVERRIDE", "v1.2.3")
	loadEnvOverrides()

	if Commit != "abcde" {
		t.Errorf("got Commit=%q, want abcde", Commit)
	}
	if Version != "v1.2.3" {
		t.Errorf("got Version=%q, want v1.2.3", Version)
	}
}
