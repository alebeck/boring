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
		if err := validateForwards(t); err != nil {
			return err
		}
		seen[t.Name] = struct{}{}
	}
	return nil
}

// validateForwards checks a tunnel's normalized Forwards slice: every forward
// must carry the addresses its mode requires, and any forward names that are
// set must be unique within the tunnel. It assumes Forwards has been populated
// (length >= 1) by config.Load.
func validateForwards(t *tunnel.Desc) error {
	seen := make(map[string]struct{})
	for i := range t.Forwards {
		f := &t.Forwards[i]
		if err := validateForwardAddresses(t.Name, f); err != nil {
			return err
		}
		if f.Name == "" {
			continue
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("tunnel %q: duplicate forward name %q",
				t.Name, f.Name)
		}
		seen[f.Name] = struct{}{}
	}
	return nil
}

// validateForwardAddresses checks that a forward defines the local/remote
// addresses required by its mode: local is required for local, remote and
// socks modes, remote is required for local, remote and socks-remote modes.
func validateForwardAddresses(tunnelName string, f *tunnel.Forward) error {
	needsLocal := f.Mode == tunnel.Local || f.Mode == tunnel.Remote ||
		f.Mode == tunnel.Socks
	if needsLocal && f.LocalAddress == "" {
		return fmt.Errorf("tunnel %q: forward in %v mode needs a local address",
			tunnelName, f.Mode.ConfigValue())
	}
	needsRemote := f.Mode == tunnel.Local || f.Mode == tunnel.Remote ||
		f.Mode == tunnel.RemoteSocks
	if needsRemote && f.RemoteAddress == "" {
		return fmt.Errorf("tunnel %q: forward in %v mode needs a remote address",
			tunnelName, f.Mode.ConfigValue())
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
