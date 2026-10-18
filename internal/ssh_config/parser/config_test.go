package parser

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func load(t *testing.T, file string) *Settings {
	t.Helper()
	s, err := Load(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		file, alias, user, key, want string
	}{
		{"config1", "wap", "", "User", "root"},
		{"config1", "wap", "", "uSER", "root"},
		{"config1", "wap", "", "Port", "22"},
		{"config1", "wap", "", "CanonicalDomains", ""},
		{"config3", "bastion.stage.i.us.example.net", "", "Port", "22"},
		{"config3", "bastion.example.net", "", "Port", "25"},
		{"config3", "10.2.3.4", "", "Port", "23"},
		{"config3", "101.2.3.4", "", "Port", "25"},
		{"config3", "20.20.20.4", "", "Port", "24"},
		{"config3", "20.20.20.20", "", "Port", "25"},
		{"config4", "wap", "", "User", "root"},
		{"config-no-ending-newline", "example", "", "Port", "4242"},
		{"extraspace", "test.test", "", "Port", "1234"},
		{"eqsign", "test.test", "", "Port", "1234"},
		{"eqsign", "test.test", "", "Port2", "5678"},
		{"quoted-identities", "hasquotedidentity", "", "IdentityFile", "/Users/testuser/.ssh/quoted_key"},
		{"quoted-identities", "hasunquotedidentity", "", "IdentityFile", "/Users/testuser/.ssh/unquoted_key"},
		{"match-directive", "testhost", "", "Port", "4567"},
		{"match-directive", "testhost", "", "HostName", "hostname"},
		{"match-directive", "wronghost", "", "HostName", ""},
		{"match-user", "testhost", "testuser", "HostName", "hostname"},
		{"match-user-first", "testhost", "", "Port", "1111"},
		{"match-final", "testhost", "", "Port", "4567"},
		{"match-final", "testhost2", "", "Port", "1234"},
		{"match-final", "testhost3", "", "Port", "3333"},
		{"modifiers", "plus", "", "Ciphers", defaults["ciphers"] + ",a,b"},
		{"modifiers", "minus", "", "Ciphers", "chacha20-poly1305@openssh.com,aes128-gcm@openssh.com,aes256-gcm@openssh.com,aes256-ctr"},
		{"modifiers", "caret", "", "Ciphers", "aes192-ctr,chacha20-poly1305@openssh.com,aes128-gcm@openssh.com,aes256-gcm@openssh.com,aes128-ctr,aes256-ctr"},
	}
	for _, tt := range tests {
		if got := load(t, tt.file).Get(tt.alias, tt.key, tt.user); got != tt.want {
			t.Errorf("%s: Get(%q, %q, %q) = %q, want %q", tt.file, tt.alias, tt.key, tt.user, got, tt.want)
		}
	}
}

func TestGetAll(t *testing.T) {
	tests := []struct {
		file, alias, key string
		want             []string
	}{
		{"identities", "hasidentity", "IdentityFile", []string{"file1"}},
		{"identities", "has2identity", "IdentityFile", []string{"f1", "f2"}},
		{"identities", "randomhost", "IdentityFile", defaultIdentities},
		{"match-user-first", "testhost", "IdentityFile", []string{"a", "b"}},
		{"config1", "wap", "UserKnownHostsFile", []string{"~/.ssh/known_hosts ~/.ssh/known_hosts2"}},
		{"config1", "wap", "CanonicalDomains", nil},
	}
	for _, tt := range tests {
		if got := load(t, tt.file).GetAll(tt.alias, tt.key, ""); !slices.Equal(got, tt.want) {
			t.Errorf("%s: GetAll(%q, %q) = %q, want %q", tt.file, tt.alias, tt.key, got, tt.want)
		}
	}
}

func TestDOSLineEndings(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	writeFile(t, cfg, "# comment\r\nHost wap\r\n  HostName wap.example.org\t\r\n\r\nHost wap2\r\n  User testuser\t\r\n")

	s, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get("wap", "HostName", ""); got != "wap.example.org" {
		t.Errorf("HostName = %q, want %q", got, "wap.example.org")
	}
	if got := s.Get("wap2", "User", ""); got != "testuser" {
		t.Errorf("User = %q, want %q", got, "testuser")
	}
}

func TestHostPatterns(t *testing.T) {
	tests := []struct {
		patterns []string
		alias    string
		want     bool
	}{
		{[]string{"*"}, "any.test", true},
		{[]string{"a", "b", "*", "c"}, "any.test", true},
		{[]string{"a", "b", "c"}, "any.test", false},
		{[]string{"any.test"}, "any1test", false},
		{[]string{"192.168.0.?"}, "192.168.0.1", true},
		{[]string{"192.168.0.?"}, "192.168.0.10", false},
		{[]string{"*.com"}, "example.com", true},
		{[]string{"*.com"}, "sub.example.com", true},
		{[]string{"*.*.com"}, "example.com", false},
		{[]string{"*.*.com"}, "sub.example.com", true},
		{[]string{"*.example.com", "!*.dialup.example.com", "foo.dialup.example.com"}, "foo.dialup.example.com", false},
		{[]string{"test.*", "!test.host"}, "test.host", false},
	}
	for _, tt := range tests {
		var h hostPatterns
		for _, p := range tt.patterns {
			h = append(h, newPattern(p))
		}
		if got := h.matches(newMatchContext(tt.alias, "")); got != tt.want {
			t.Errorf("%q matches %q: got %v, want %v", tt.patterns, tt.alias, got, tt.want)
		}
	}
}

func TestInclude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".ssh", "relative"), "Host relative\n  Port 1111\n")
	writeFile(t, filepath.Join(home, "tilde"), "Host tilde\n  Port 2222\n")
	abs := filepath.Join(home, "absolute")
	writeFile(t, abs, "Host absolute\n  Port 3333\n")
	cfg := filepath.Join(home, "config")
	writeFile(t, cfg, "Include relative ~/tilde "+abs+"\n")

	s, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for alias, want := range map[string]string{"relative": "1111", "tilde": "2222", "absolute": "3333"} {
		if got := s.Get(alias, "Port", ""); got != want {
			t.Errorf("Get(%q, Port) = %q, want %q", alias, got, want)
		}
	}
}

func TestIncludeRecursive(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	writeFile(t, cfg, "Include "+cfg+"\n")
	if _, err := Load(cfg); !errors.Is(err, errDepthExceeded) {
		t.Errorf("expected errDepthExceeded, got %v", err)
	}
}
