package buildinfo

import "os"

var (
	// Commit is the 5-character commit string injected at build time
	Commit string
	// Tag of the binary
	Tag string
)

func init() {
	if c := os.Getenv("BORING_COMMIT_OVERRIDE"); c != "" {
		Commit = c
	}
	if t := os.Getenv("BORING_TAG_OVERRIDE"); t != "" {
		Tag = t
	}
}
