package buildinfo

import "os"

var (
	// Commit is the 5-character commit string injected at build time
	Commit string
	// Version of the binary
	Version string
)

func init() {
	if c := os.Getenv("BORING_COMMIT_OVERRIDE"); c != "" {
		Commit = c
	}
	if v := os.Getenv("BORING_VERSION_OVERRIDE"); v != "" {
		Version = v
	}
}
