package tunnel

import (
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/paths"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	systemConfigPath  = "/etc/ssh/ssh_config"
	sshConnTimeout    = 10 * time.Second
	maxJumpRecursions = 20
)

var (
	userConfigPath = paths.ReplaceTilde("~/.ssh/config")
	defaultKeys    = []string{"~/.ssh/id_rsa", "~/.ssh/id_ecdsa", "~/.ssh/id_ed25519"}
	algos          = []string{"Ciphers", "MACs", "HostKeyAlgorithms", "KexAlgorithms"}
)

type proxyJump struct {
	host string
	user string
	port int
}

type sshConfig struct {
	user            string
	hostName        string
	port            int
	identityFiles   []string
	knownHostsFiles []string
	ciphers         []string
	macs            []string
	hostKeyAlgos    []string
	kexAlgos        []string
	jumps           []*proxyJump
}

// connSpec holds data needed to establish an SSH connection
type connSpec struct {
	hostName string
	port     int
	*ssh.ClientConfig
}

// sshConfigSpec is a thin wrapper around kevinburke/ssh_config, with
// the intent to make the UserConfig (here sshConfigSpec) object accessible.
// We need this so we can reload the config at every tunnel opening, and not
// just once at application start.
type sshConfigSpec struct {
	userConfigSpec   *ssh_config.Config
	systemConfigSpec *ssh_config.Config
}

func parseProxyJump(s string) (*proxyJump, error) {
	// Format: [user@]host[:port]
	// TODO: use regex?
	var portInt int
	var err error
	host, port, _ := strings.Cut(s, ":")
	if port != "" {
		if portInt, err = strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("could not parse port: %v", err)
		}
	}
	user, host, fnd := strings.Cut(host, "@")
	if !fnd {
		user, host = host, user
	}
	return &proxyJump{host: host, user: user, port: portInt}, nil
}

func parseSSHConfig(alias string) (*sshConfig, error) {
	d, err := makeSSHConfigDesc()
	if err != nil {
		return nil, err
	}

	c := &sshConfig{}

	c.identityFiles = d.GetAll(alias, "IdentityFile")

	// Known hosts
	hosts := d.GetAll(alias, "GlobalKnownHostsFile")
	hosts = append(hosts, d.GetAll(alias, "UserKnownHostsFile")...)
	for _, h := range hosts {
		c.knownHostsFiles = append(c.knownHostsFiles, strings.Split(h, " ")...)
	}

	c.ciphers = split(d.Get(alias, "Ciphers"))
	c.macs = split(d.Get(alias, "MACs"))
	c.hostKeyAlgos = split(d.Get(alias, "HostKeyAlgorithms"))
	c.kexAlgos = split(d.Get(alias, "KexAlgorithms"))

	c.user = d.Get(alias, "User")
	c.port, _ = strconv.Atoi(d.Get(alias, "Port"))
	c.hostName = d.Get(alias, "HostName")

	// Jump hosts
	if pj := d.Get(alias, "ProxyJump"); pj != "" {
		for _, j := range split(pj) {
			jump, err := parseProxyJump(j)
			if err != nil {
				return nil, fmt.Errorf("could not parse jump host: %v", err)
			}
			c.jumps = append(c.jumps, jump)
		}
	}

	return c, nil
}

// toConnSpecs creates an ordered series of connSpecs from an sshConfig
func (sc *sshConfig) toConnSpecs() ([]connSpec, error) {
	return sc.toConnSpecsImpl(false, 0)
}

func (sc *sshConfig) toConnSpecsImpl(ignoreJumps bool, depth int) ([]connSpec, error) {
	if depth > maxJumpRecursions {
		return nil, fmt.Errorf("maximum jump recursions exceeded")
	}

	if err := sc.validate(); err != nil {
		return nil, err
	}

	if ignoreJumps {
		sc.jumps = nil
	}

	var s []connSpec

	for i, j := range sc.jumps {
		jc, err := parseSSHConfig(j.host)
		if err != nil {
			return nil, fmt.Errorf("could not parse SSH config for %v: %v", j.host, err)
		}

		// Replace jump user & port if provided
		if j.user != "" {
			jc.user = j.user
		}
		if j.port != 0 {
			jc.port = j.port
		}

		// Recursively connect to first jump host, ignore jumps for subsequent connections;
		// this corresponds to ssh(1) behavior
		js, err := jc.toConnSpecsImpl(i != 0, depth+1)
		if err != nil {
			return nil, err
		}
		s = append(s, js...)
	}

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

	addKeyFiles(sc.identityFiles)

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
			return nil, fmt.Errorf("no key files found.")
		}
	}
	log.Debugf("Trying %d key file(s)", len(signers))

	var hosts []string
	for _, k := range sc.knownHostsFiles {
		k = paths.ReplaceTilde(k)
		if _, err := os.Stat(k); err == nil {
			hosts = append(hosts, k)
		}
	}
	knownHostsCallback, err := knownhosts.New(hosts...)
	if err != nil {
		return nil, err
	}

	clientConf := &ssh.ClientConfig{
		Config: ssh.Config{
			Ciphers:      sc.ciphers,
			KeyExchanges: sc.kexAlgos,
			MACs:         sc.macs,
		},
		User:              sc.user,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signers...)},
		HostKeyAlgorithms: sc.hostKeyAlgos,
		HostKeyCallback:   knownHostsCallback,
		Timeout:           sshConnTimeout,
	}

	new := connSpec{hostName: sc.hostName, port: sc.port, ClientConfig: clientConf}
	s = append(s, new)

	return s, nil
}

func (sc *sshConfig) validate() error {
	if sc.hostName == "" {
		return fmt.Errorf("no host specified.")
	}
	if sc.user == "" {
		return fmt.Errorf("no user specified.")
	}
	if sc.port == 0 {
		return fmt.Errorf("no port specified.")
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

func makeSSHConfigDesc() (*sshConfigSpec, error) {
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
	return ssh_config.Default(key)
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

	// check for special symbols within algorithm specifications
	if slices.Contains(algos, key) {
		val = processAlgos(val, key)
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

func processAlgos(v, key string) string {
	if !(strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") ||
		strings.HasPrefix(v, "^")) {
		return v
	}

	cur := strings.Split(v[1:], ",")
	def := strings.Split(ssh_config.Default(key), ",")
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

func split(s string) []string {
	return strings.Split(s, ",")
}
