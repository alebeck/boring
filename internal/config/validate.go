package config

import (
	"fmt"
	"strings"

	"github.com/alebeck/boring/internal/tunnel"
)

// Validate checks that a list of tunnel descriptions satisfies the naming
// rules enforced by boring: tunnel names must be unique and non-empty, and
// neither tunnel nor group names may contain spaces, start with special
// characters, or contain glob characters. It returns the first violation
// encountered, or nil if all tunnels are valid.
func Validate(tunnels []tunnel.Desc) error {
	seen := make(map[string]struct{})
	for i := range tunnels {
		t := &tunnels[i]
		if _, exists := seen[t.Name]; exists {
			return fmt.Errorf("found duplicated tunnel name '%v'", t.Name)
		}
		if t.Name == "" || strings.Contains(t.Name, " ") ||
			specialPrefix(t.Name) || containsGlob(t.Name) {
			return fmt.Errorf("tunnel names cannot be empty, contain spaces,"+
				" start with special characters, or contain glob characters \"*?[\"."+
				" Found '%v'", t.Name)
		}
		if t.Group != "" && (strings.Contains(t.Group, " ") ||
			specialPrefix(t.Group) || containsGlob(t.Group)) {
			return fmt.Errorf("group names cannot contain spaces,"+
				" start with special characters, or contain glob characters \"*?[\"."+
				" Found '%v'", t.Group)
		}
		seen[t.Name] = struct{}{}
	}
	return nil
}

func specialPrefix(s string) bool {
	if s == "" {
		return false
	}
	firstChar := s[0]
	return !(firstChar >= 'A' && firstChar <= 'Z' ||
		firstChar >= 'a' && firstChar <= 'z' ||
		firstChar >= '0' && firstChar <= '9')
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
