package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func ReplaceTilde(path string) string {
	home := os.Getenv("HOME")
	if path == "~" {
		return home
	} else if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
