package tunnel

import (
	"fmt"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/agent"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/paths"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
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

type jumpSpec struct {
	host string
	user string
	port int
}

type keyCheck int

const (
	// Reject unknown hosts by default, this corresponds to "yes" and "ask"
	// options in ssh_config. Note that "ask" is treated the same as "yes",
	// as boring is not meant to be interactive.
	strict keyCheck = iota
	// Accepts all hosts, this corresponds to "no" and "off" options
	off
	// TODO: support "accept-new" option?
)

type sshConfig struct {
	user            string
	hostName        string
	port            int
	keyCheck        keyCheck
	identityFiles   []string
	knownHostsFiles []string
	ciphers         []string
	macs            []string
	hostKeyAlgos    []string
	kexAlgos        []string
	jumps           []*jumpSpec
}

// jump holds information needed to establish a single SSH jump
type jump struct {
	hostName string
	port     int
	*ssh.ClientConfig
}

type subst map[string]string

var (
	hostnameTokens  = []string{"%%", "%h"}
	proxyTokens     = []string{"%%", "h", "%n", "%p", "%r"}
	identFileTokens = []string{
		"%%", "%d", "%h", "%i", "%j", "%k",
		"%L", "%l", "%n", "%p", "%r", "%u",
	}
)

// sshConfigSpec is a thin wrapper around kevinburke/ssh_config, with
// the intent to make the UserConfig (here sshConfigSpec) object accessible.
// We need this so we can reload the config at every tunnel opening, and not
// just once at application start.
type sshConfigSpec struct {
	userConfigSpec   *ssh_config.Config
	systemConfigSpec *ssh_config.Config
}

func makeSubst(alias string) subst {
	s := map[string]string{
		"%%": "%",
		"%p": "22",
		"%h": alias,
		"%n": alias,
	}
	if u, err := user.Current(); err == nil {
		s["%u"] = u.Username
		s["%d"] = u.HomeDir
		s["%i"] = u.Uid
	}
	if h, err := os.Hostname(); err == nil {
		s["%L"] = h
		// TODO: %l (FQDN)
	}
	return s
}

func (s subst) apply(str string, keys []string) string {
	if !strings.Contains(str, "%") {
		return str
	}
	for _, k := range keys {
		if r, ok := s[k]; ok {
			str = strings.ReplaceAll(str, k, r)
		}
	}
	return str
}

func (s subst) applyAll(strs []string, keys []string) []string {
	var out []string
	for _, str := range strs {
		out = append(out, s.apply(str, keys))
	}
	return out
}

func parseProxyJump(s string) (*jumpSpec, error) {
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
	return &jumpSpec{host: host, user: user, port: portInt}, nil
}

func parseSSHConfig(alias string) (*sshConfig, error) {
	d, err := makeSSHConfigSpec()
	if err != nil {
		return nil, err
	}

	c := &sshConfig{}
	sub := makeSubst(alias)

	c.hostName = sub.apply(d.Get(alias, "HostName"), hostnameTokens)
	sub["%h"] = c.hostName

	c.user = d.Get(alias, "User")
	sub["%r"] = c.user
	c.port, _ = strconv.Atoi(d.Get(alias, "Port"))
	sub["%p"] = fmt.Sprintf("%d", c.port)

	s := d.Get(alias, "StrictHostKeyChecking")
	if s == "no" || s == "off" {
		c.keyCheck = off
	} else if s == "accept-new" {
		log.Warningf(
			"StrictHostKeyChecking 'accept-new' not supported, using 'yes'")
	} else if s != "yes" && s != "ask" {
		return nil, fmt.Errorf(
			"unsupported StrictHostKeyChecking option '%v'", s)
	}

	c.ciphers = split(d.Get(alias, "Ciphers"))
	c.macs = split(d.Get(alias, "MACs"))
	c.hostKeyAlgos = split(d.Get(alias, "HostKeyAlgorithms"))
	c.kexAlgos = split(d.Get(alias, "KexAlgorithms"))

	// Jump hosts
	pj := sub.apply(d.Get(alias, "ProxyJump"), proxyTokens)
	sub["%j"] = pj
	if pj != "" {
		for _, j := range split(pj) {
			jump, err := parseProxyJump(j)
			if err != nil {
				return nil, fmt.Errorf("could not parse jump host: %v", err)
			}
			c.jumps = append(c.jumps, jump)
		}
	}

	c.identityFiles = sub.applyAll(d.GetAll(alias, "IdentityFile"), identFileTokens)

	// Known hosts
	hosts := d.GetAll(alias, "GlobalKnownHostsFile")
	hosts = append(hosts, sub.applyAll(d.GetAll(alias, "UserKnownHostsFile"), identFileTokens)...)
	for _, h := range hosts {
		c.knownHostsFiles = append(c.knownHostsFiles, strings.Split(h, " ")...)
	}

	return c, nil
}

// toJumps creates an ordered series of jumps from an sshConfig
func (sc *sshConfig) toJumps() ([]jump, error) {
	return sc.toJumpsImpl(false, 0)
}

func (sc *sshConfig) toJumpsImpl(ignoreIntermediate bool, depth int) ([]jump, error) {
	if depth > maxJumpRecursions {
		return nil, fmt.Errorf("maximum jump recursions exceeded")
	}

	if err := sc.validate(); err != nil {
		return nil, err
	}

	if ignoreIntermediate {
		sc.jumps = nil
	}

	var s []jump

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
		js, err := jc.toJumpsImpl(i != 0, depth+1)
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
		agentSigners, err := agent.GetSigners()
		if err != nil {
			log.Warningf("Unable to get keys from ssh-agent: %v", err)
		}
		signers = append(signers, agentSigners...)
		log.Debugf("Added %d signers from ssh-agent", len(agentSigners))

		if len(signers) == 0 {
			return nil, fmt.Errorf("no key files found")
		}
	}
	log.Debugf("Trying %d key file(s)", len(signers))

	var keyCallback ssh.HostKeyCallback
	if sc.keyCheck == strict {
		var hosts []string
		for _, k := range sc.knownHostsFiles {
			k = paths.ReplaceTilde(k)
			if _, err := os.Stat(k); err != nil {
				log.Debugf("could not open known hosts file %v: %v", k, err)
				continue
			}
			hosts = append(hosts, k)
		}
		var err error
		if keyCallback, err = knownhosts.New(hosts...); err != nil {
			return nil, fmt.Errorf("knownhosts: %v", err)
		}
	} else if sc.keyCheck == off {
		keyCallback = ssh.InsecureIgnoreHostKey()
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
		HostKeyCallback:   keyCallback,
		Timeout:           sshConnTimeout,
	}

	new := jump{hostName: sc.hostName, port: sc.port, ClientConfig: clientConf}
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
