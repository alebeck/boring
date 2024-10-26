package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/paths"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const sshConnTimeout = 10 * time.Second

var defaultKeys = []string{"~/.ssh/id_rsa", "~/.ssh/id_ecdsa", "~/.ssh/id_ed25519"}

type runConfig struct {
	localAddress    string
	remoteAddress   string
	localNet        string // tcp, unix
	remoteNet       string // tcp, unix
	hostName        string
	user            string
	port            int
	identityFiles   []string
	knownHostsFiles []string
	ciphers         []string
	macs            []string
	hostKeyAlgos    []string
	kexAlgos        []string
	clientConfig    *ssh.ClientConfig
}

func (t *Tunnel) makeRunConfig() error {
	rc := runConfig{}

	// Fill in rc's values based on ssh config
	if err := rc.parseSSHConf(t.Host); err != nil {
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
		rc.identityFiles = []string{t.IdentityFile}
	}

	// If t.Host could not be resolved from ssh config, take it literally
	if rc.hostName == "" {
		rc.hostName = t.Host
	}

	// Use $USER if still no user specified
	if rc.user == "" {
		if u, err := user.Current(); err == nil {
			rc.user = u.Username
		}
	}

	if err := validate(&rc); err != nil {
		return err
	}

	// Make SSH client config
	if err := rc.makeClientConfig(); err != nil {
		return fmt.Errorf("client config: %v", err)
	}

	var err error
	short := t.Mode == Remote || t.Mode == RemoteSocks
	rc.remoteAddress, rc.remoteNet, err = parseAddr(string(t.RemoteAddress), short)
	if err != nil {
		return fmt.Errorf("remote address: %v", err)
	}
	rc.localAddress, rc.localNet, err = parseAddr(string(t.LocalAddress), !short)
	if err != nil {
		return fmt.Errorf("local address: %v", err)
	}

	t.rc = &rc
	return nil
}

func (rc *runConfig) parseSSHConf(alias string) error {
	c, err := makeSSHConfig()
	if err != nil {
		return err
	}

	rc.identityFiles = c.GetAll(alias, "IdentityFile")

	hosts := c.GetAll(alias, "GlobalKnownHostsFile")
	hosts = append(hosts, c.GetAll(alias, "UserKnownHostsFile")...)
	for _, h := range hosts {
		rc.knownHostsFiles = append(rc.knownHostsFiles, strings.Split(h, " ")...)
	}

	rc.ciphers = split(c.Get(alias, "Ciphers"))
	rc.macs = split(c.Get(alias, "MACs"))
	rc.hostKeyAlgos = split(c.Get(alias, "HostKeyAlgorithms"))
	rc.kexAlgos = split(c.Get(alias, "KexAlgorithms"))

	rc.user = c.Get(alias, "User")
	rc.port, _ = strconv.Atoi(c.Get(alias, "Port"))
	rc.hostName = c.Get(alias, "HostName")
	return nil
}

func validate(rc *runConfig) error {
	if rc.hostName == "" {
		return fmt.Errorf("no host specified.")
	}
	if rc.user == "" {
		return fmt.Errorf("no user specified.")
	}
	if rc.port == 0 {
		return fmt.Errorf("no port specified.")
	}
	return nil
}

func (rc *runConfig) makeClientConfig() error {
	var signers []ssh.Signer
	addKeyFiles := func(files []string) {
		for _, f := range files {
			s, err := loadKey(f)
			if err != nil {
				log.Warningf("key file %v could not be added: %v", f, err)
				continue
			}
			signers = append(signers, *s)
		}
	}

	addKeyFiles(rc.identityFiles)

	if len(signers) == 0 {
		log.Warningf("No key files specified, trying default ones")
		addKeyFiles(defaultKeys)

		// Also add potential keys exposed by ssh-agent
		agentSigners, err := getAgentSigners()
		if err != nil {
			log.Warningf("Unable to get keys from ssh-agent: %v", err)
		}
		signers = append(signers, agentSigners...)

		if len(signers) == 0 {
			return fmt.Errorf("no key files found.")
		}
	}
	log.Debugf("Trying %d key file(s)", len(signers))

	var hosts []string
	for _, k := range rc.knownHostsFiles {
		k = paths.ReplaceTilde(k)
		if _, err := os.Stat(k); err == nil {
			hosts = append(hosts, k)
		}
	}
	knownHostsCallback, err := knownhosts.New(hosts...)
	if err != nil {
		return err
	}

	rc.clientConfig = &ssh.ClientConfig{
		Config: ssh.Config{
			Ciphers:      rc.ciphers,
			KeyExchanges: rc.kexAlgos,
			MACs:         rc.macs,
		},
		User:              rc.user,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signers...)},
		HostKeyAlgorithms: rc.hostKeyAlgos,
		HostKeyCallback:   knownHostsCallback,
		Timeout:           sshConnTimeout,
	}
	return nil
}

func loadKey(path string) (*ssh.Signer, error) {
	if path == "" {
		return nil, fmt.Errorf("no key specified")
	}
	key, err := os.ReadFile(paths.ReplaceTilde(path))
	if err != nil {
		return nil, fmt.Errorf("could not read key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("could not parse key: %v", err)
	}
	return &signer, nil
}

func getAgentSigners() ([]ssh.Signer, error) {
	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, fmt.Errorf("could not dial agent: %v", err)
	}
	c := agent.NewClient(sock)
	return c.Signers()
}

func parseAddr(addr string, allowShort bool) (string, string, error) {
	if _, err := strconv.Atoi(addr); err == nil {
		// addr is a tcp port number
		if !allowShort {
			return "", "", fmt.Errorf("bad remote forwarding specification")
		}
		return "localhost:" + addr, "tcp", nil
	} else if strings.Contains(addr, ":") {
		// addr is a full tcp address
		return addr, "tcp", nil
	}
	// it's a unix socket address
	return addr, "unix", nil
}

func split(s string) []string {
	return strings.Split(s, ",")
}
