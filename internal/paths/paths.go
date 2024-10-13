package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReplaceTilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("Home directory not found: %v", err))
	}
	if path == "~" {
		return home
	} else if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
