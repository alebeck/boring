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

// ExpandEnv replaces ${VAR} with the value of the environment variable VAR,
// like ssh_config(5). Unlike os.ExpandEnv, $VAR is kept as is and unset
// variables are an error.
func ExpandEnv(s string) (string, error) {
	var b strings.Builder
	for {
		before, after, found := strings.Cut(s, "${")
		b.WriteString(before)
		if !found {
			return b.String(), nil
		}
		name, rest, ok := strings.Cut(after, "}")
		if !ok {
			return "", fmt.Errorf("missing '}' in %q", s)
		}
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", name)
		}
		b.WriteString(val)
		s = rest
	}
}
