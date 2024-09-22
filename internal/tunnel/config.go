package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const sshPort = 22
const sshConnTimeout = 10 * time.Second

type RunConfig struct {
	localAddress  string
	remoteAddress string
	hostName      string
	user          string
	port          int
	identityFile  string
	clientConfig  *ssh.ClientConfig
}

func (t *Tunnel) makeRunConfig() error {
	rc := RunConfig{}

	// Fill in rc's values based on ssh config
	if err := parseSSHConf(t.Host, &rc); err != nil {
		return fmt.Errorf("could not parse SSH config: %v", err)
	}

	// Override values which were manually set by user
	if t.User != "" {
		rc.user = t.User
	}
	if t.Port != 0 {
		rc.port = t.Port
	}
	if t.IdentityFile != "" {
		rc.identityFile = t.IdentityFile
	}

	// If port is still not set, use default
	if rc.port == 0 {
		rc.port = sshPort
	}

	// If t.Host could not be resolved from ssh config, take it literally
	if rc.hostName == "" {
		rc.hostName = t.Host
	}

	if err := validate(&rc); err != nil {
		return err
	}

	// Make SSH client config
	var err error
	if rc.clientConfig, err = makeClientConfig(rc.user, rc.identityFile); err != nil {
		return fmt.Errorf("could not make client config: %v", err)
	}

	rc.remoteAddress = t.RemoteAddress
	rc.localAddress = t.LocalAddress
	if !strings.Contains(t.LocalAddress, ":") {
		rc.localAddress = "localhost:" + rc.localAddress
	}

	t.rc = &rc
	return nil
}

func parseSSHConf(alias string, rc *RunConfig) error {
	// TODO check /etc/ssh/ssh_config if user one does not exist
	sshConfFile, err := os.Open(sshConfigPath("config"))
	if err != nil {
		return nil
	}
	sshConf, err := ssh_config.Decode(sshConfFile)
	if err != nil {
		return fmt.Errorf("could not decode ssh config: %v", err)
	}
	sshConfFile.Close()

	// TODO: allow multiple key files
	rc.identityFile, _ = sshConf.Get(alias, "IdentityFile")

	rc.user, _ = sshConf.Get(alias, "User")

	port, _ := sshConf.Get(alias, "Port")
	rc.port, _ = strconv.Atoi(port)

	rc.hostName, _ = sshConf.Get(alias, "HostName")

	return nil
}

func validate(rc *RunConfig) error {
	if rc.hostName == "" {
		return fmt.Errorf("no host specified.")
	}
	if rc.identityFile == "" {
		return fmt.Errorf("no identity file specified.")
	}
	if rc.user == "" {
		return fmt.Errorf("no user specified.")
	}
	if rc.port == 0 {
		return fmt.Errorf("no port specified.")
	}
	return nil
}

func makeClientConfig(user, identityFile string) (*ssh.ClientConfig, error) {
	key, err := os.ReadFile(fillHome(identityFile))
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}
	knownHostsCallback, err := knownhosts.New(sshConfigPath("known_hosts"))
	if err != nil {
		return nil, err
	}

	// TODO: timeout
	conf := ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: knownHostsCallback,
		Timeout:         sshConnTimeout,
	}
	return &conf, nil
}

func sshConfigPath(filename string) string {
	return filepath.Join(os.Getenv("HOME"), ".ssh", filename)
}

func fillHome(path string) string {
	home := os.Getenv("HOME")
	if path == "~" {
		return home
	} else if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
