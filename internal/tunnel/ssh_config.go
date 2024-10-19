package tunnel

import (
	"os"

	"github.com/alebeck/boring/internal/paths"
	"github.com/kevinburke/ssh_config"
)

var (
	userConfigPath   = paths.ReplaceTilde("~/.ssh/config")
	systemConfigPath = "/etc/ssh/ssh_config"
)

// sshConfig (and the following functions) are a thin wrapper around
// excellent kevinburke/ssh_config, with the intend to make the
// UserConfig (here sshConfig) object accessible. We need this so we
// can reload the config at every tunnel opening, and not just once
// at application start.
type sshConfig struct {
	userConfig   *ssh_config.Config
	systemConfig *ssh_config.Config
}

func makeSSHConfig() (*sshConfig, error) {
	uc, err := parse(userConfigPath)
	if err != nil {
		return nil, err
	}
	sc, err := parse(systemConfigPath)
	if err != nil {
		return nil, err
	}
	c := &sshConfig{userConfig: uc, systemConfig: sc}
	return c, nil
}

// (c) kevinburke/ssh_config
func (c *sshConfig) Get(alias, key string) string {
	val, err := findVal(c.userConfig, alias, key)
	if err != nil || val != "" {
		return val
	}
	val2, err2 := findVal(c.systemConfig, alias, key)
	if err2 != nil || val2 != "" {
		return val2
	}
	return ssh_config.Default(key)
}

// (c) kevinburke/ssh_config
func (c *sshConfig) GetAll(alias, key string) []string {
	val, err := findAll(c.userConfig, alias, key)
	if err != nil || val != nil {
		return val
	}
	val2, err2 := findAll(c.systemConfig, alias, key)
	if err2 != nil || val2 != nil {
		return val2
	}
	if key == "IdentityFile" {
		// The original implementation returned outdated SSH1 identites
		// file, so we instead return nothing if no files were specified.
		// We would return the SSH2 default files, but boring has no way
		// to determine whether those are specified or default ones, and
		// we only include agent signers in case no files are specified.
		return []string{}
	}
	if def := ssh_config.Default(key); def != "" {
		return []string{def}
	}
	return []string{}
}

// (c) kevinburke/ssh_config
func findVal(c *ssh_config.Config, alias, key string) (string, error) {
	if c == nil {
		return "", nil
	}
	val, err := c.Get(alias, key)
	if err != nil || val == "" {
		return "", err
	}
	return val, nil
}

// (c) kevinburke/ssh_config
func findAll(c *ssh_config.Config, alias, key string) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	return c.GetAll(alias, key)
}

func parse(path string) (*ssh_config.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	c, err := ssh_config.Decode(f)
	if err != nil {
		return nil, err
	}
	return c, nil
}
