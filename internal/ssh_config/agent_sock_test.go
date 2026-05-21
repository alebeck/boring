package ssh_config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentSock(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	t.Setenv("SSH_AUTH_SOCK", "/run/auth.sock")
	t.Setenv("MY_AGENT", "/run/custom.sock")

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"absent falls back to SSH_AUTH_SOCK", "", "/run/auth.sock"},
		{"literal SSH_AUTH_SOCK", "SSH_AUTH_SOCK", "/run/auth.sock"},
		{"none disables the agent", "none", ""},
		{"env var reference", "$MY_AGENT", "/run/custom.sock"},
		{"plain path", "/tmp/agent.sock", "/tmp/agent.sock"},
		{"quoted path with a space", `"/tmp/a b/agent.sock"`, "/tmp/a b/agent.sock"},
		{"tilde is expanded", "~/agent.sock", filepath.Join(home, "agent.sock")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveAgentSock(tc.raw); got != tc.want {
				t.Fatalf("resolveAgentSock(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestParseSSHConfigIdentityAgent locks the fix for the bug where boring
// ignored the IdentityAgent directive and always used $SSH_AUTH_SOCK — which
// breaks the standard 1Password SSH-agent setup on macOS.
func TestParseSSHConfigIdentityAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/run/fallback.sock")

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "IdentityAgent directive is honored",
			content: "Host h\n\tIdentityAgent /tmp/onepassword.sock\n",
			want:    "/tmp/onepassword.sock",
		},
		{
			name:    "no IdentityAgent falls back to SSH_AUTH_SOCK",
			content: "Host h\n\tUser bob\n",
			want:    "/run/fallback.sock",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(cfg, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			old := overrideConfig
			overrideConfig = cfg
			t.Cleanup(func() { overrideConfig = old })

			c, err := ParseSSHConfig("h", "")
			if err != nil {
				t.Fatalf("ParseSSHConfig: %v", err)
			}
			if c.AgentSock != tc.want {
				t.Fatalf("AgentSock = %q, want %q", c.AgentSock, tc.want)
			}
		})
	}
}
