package ssh_config

import (
	"github.com/alebeck/boring/internal/paths"
	"os"
	"slices"
	"strings"

	ossh_config "github.com/kevinburke/ssh_config"
)

const systemConfigPath = "/etc/ssh/ssh_config"

var (
	userConfigPath = paths.ReplaceTilde("~/.ssh/config")
	algoSpecifiers = []string{"Ciphers", "MACs", "HostKeyAlgorithms", "KexAlgorithms"}
)

// sshConfigSpec is a thin wrapper around kevinburke/ssh_config, with
// the intent to make the UserConfig (here sshConfigSpec) object accessible.
// We need this so we can reload the config at every tunnel opening, and not
// just once at application start.
type sshConfigSpec struct {
	userConfigSpec   *ossh_config.Config
	systemConfigSpec *ossh_config.Config
}

func makeSSHConfigSpec() (*sshConfigSpec, error) {
	uc, err := parse(userConfigPath)
	if err != nil {
		return nil, err
	}
	sc, err := parse(systemConfigPath)
	if err != nil {
		return nil, err
	}
	c := &sshConfigSpec{userConfigSpec: uc, systemConfigSpec: sc}
	return c, nil
}

// (c) kevinburke/ssh_config
func (c *sshConfigSpec) Get(alias, key string) string {
	val, err := findVal(c.userConfigSpec, alias, key)
	if err != nil || val != "" {
		return val
	}
	val2, err2 := findVal(c.systemConfigSpec, alias, key)
	if err2 != nil || val2 != "" {
		return val2
	}
	return ossh_config.Default(key)
}

// (c) kevinburke/ssh_config
func (c *sshConfigSpec) GetAll(alias, key string) []string {
	val, err := findAll(c.userConfigSpec, alias, key)
	if err != nil || val != nil {
		return val
	}
	val2, err2 := findAll(c.systemConfigSpec, alias, key)
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
	if def := ossh_config.Default(key); def != "" {
		return []string{def}
	}
	return []string{}
}

// (c) kevinburke/ssh_config
func findVal(c *ossh_config.Config, alias, key string) (string, error) {
	if c == nil {
		return "", nil
	}
	val, err := c.Get(alias, key)
	if err != nil || val == "" {
		return "", err
	}

	// check for special symbols within algorithm specifications
	if slices.Contains(algoSpecifiers, key) {
		val = processAlgos(val, key)
	}

	return val, nil
}

// (c) kevinburke/ssh_config
func findAll(c *ossh_config.Config, alias, key string) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	return c.GetAll(alias, key)
}

func parse(path string) (*ossh_config.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	c, err := ossh_config.Decode(f)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func processAlgos(v, key string) string {
	if !(strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") ||
		strings.HasPrefix(v, "^")) {
		return v
	}

	cur := strings.Split(v[1:], ",")
	def := strings.Split(ossh_config.Default(key), ",")
	var out []string

	switch v[0] {
	case '+':
		out = append(def, cur...)
	case '-':
		out = make([]string, 0, len(def))
		for _, a := range def {
			if !slices.Contains(cur, a) {
				out = append(out, a)
			}
		}
	case '^':
		out = make([]string, 0, len(def)+len(cur))
		for _, a := range cur {
			if slices.Contains(def, a) {
				out = append(out, a)
			}
		}
		for _, a := range def {
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}

	return strings.Join(out, ",")
}
