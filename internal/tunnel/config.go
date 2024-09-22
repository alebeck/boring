package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (t *Tunnel) parseSSHConf() error {
	// TODO check /etc/ssh/ssh_config if user one does not exist
	sshConfFile, err := os.Open(sshConfigPath("config"))
	if err != nil {
		return nil
	}
	sshConf, err := ssh_config.Decode(sshConfFile)
	if err != nil {
		return err // TODO
	}
	sshConfFile.Close()

	// TODO: allow multiple key files
	if t.IdentityFile == "" {
		t.IdentityFile, _ = sshConf.Get(t.Host, "IdentityFile")
	}
	if t.User == "" {
		t.User, _ = sshConf.Get(t.Host, "User")
	}
	if t.Port == 0 {
		port, _ := sshConf.Get(t.Host, "Port")
		if t.Port, _ = strconv.Atoi(port); t.Port == 0 {
			// Use SSH standard port
			t.Port = 22
		}
	}
	if hostName, _ := sshConf.Get(t.Host, "HostName"); hostName != "" {
		t.HostName = hostName
	}

	return nil
}

func (t *Tunnel) validate() error {
	if t.Host == "" && t.HostName == "" {
		return fmt.Errorf("no host specified.")
	}
	if t.IdentityFile == "" {
		return fmt.Errorf("no identity file specified.")
	}
	if t.User == "" {
		return fmt.Errorf("no user specified.")
	}
	if t.Port == 0 {
		return fmt.Errorf("no port specified.")
	}
	return nil
}

func (t *Tunnel) makeClientConf() (*ssh.ClientConfig, error) {
	// Private key file and known hosts
	key, err := os.ReadFile(fillHome(t.IdentityFile))
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
	conf := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback:   knownHostsCallback,
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
	}
	return conf, nil
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
