package ssh_config

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/agent"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/paths"
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
)

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

// Jump holds information needed to establish a single SSH jump
type Jump struct {
	HostName string
	Port     int
	*ssh.ClientConfig
}

// sshConfig represents an SSH config read from, e.g., ~/.ssh/config
type sshConfig struct {
	Alias           string
	User            string
	HostName        string
	Port            int
	KeyCheck        keyCheck
	IdentityFiles   []string
	KnownHostsFiles []string
	Ciphers         []string
	Macs            []string
	HostKeyAlgos    []string
	KexAlgos        []string
	Jumps           []*jumpSpec
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

func ParseSSHConfig(alias string) (*sshConfig, error) {
	d, err := makeSSHConfigSpec()
	if err != nil {
		return nil, err
	}

	c := &sshConfig{Alias: alias}
	sub := makeSubst(alias)

	c.HostName = sub.apply(d.Get(alias, "HostName"), hostnameTokens)
	sub["%h"] = c.HostName

	c.User = d.Get(alias, "User")
	sub["%r"] = c.User
	c.Port, _ = strconv.Atoi(d.Get(alias, "Port"))
	sub["%p"] = fmt.Sprintf("%d", c.Port)

	s := d.Get(alias, "StrictHostKeyChecking")
	if s == "no" || s == "off" {
		c.KeyCheck = off
	} else if s == "accept-new" {
		log.Warningf(
			"StrictHostKeyChecking 'accept-new' not supported, using 'yes'")
	} else if s != "yes" && s != "ask" {
		return nil, fmt.Errorf(
			"unsupported StrictHostKeyChecking option '%v'", s)
	}

	c.Ciphers = split(d.Get(alias, "Ciphers"))
	c.Macs = split(d.Get(alias, "MACs"))
	c.HostKeyAlgos = split(d.Get(alias, "HostKeyAlgorithms"))
	c.KexAlgos = split(d.Get(alias, "KexAlgorithms"))

	// Jump hosts
	pj := sub.apply(d.Get(alias, "ProxyJump"), proxyTokens)
	sub["%j"] = pj
	if pj != "" {
		for _, j := range split(pj) {
			jump, err := parseProxyJump(j)
			if err != nil {
				return nil, fmt.Errorf("could not parse jump host: %v", err)
			}
			c.Jumps = append(c.Jumps, jump)
		}
	}

	c.IdentityFiles = sub.applyAll(d.GetAll(alias, "IdentityFile"), identFileTokens)

	// Known hosts
	hosts := d.GetAll(alias, "GlobalKnownHostsFile")
	hosts = append(hosts, sub.applyAll(d.GetAll(alias, "UserKnownHostsFile"), identFileTokens)...)
	for _, h := range hosts {
		c.KnownHostsFiles = append(c.KnownHostsFiles, strings.Split(h, " ")...)
	}

	return c, nil
}

// toJumps creates an ordered series of jumps from an sshConfig
func (sc *sshConfig) ToJumps() ([]Jump, error) {
	return sc.toJumpsImpl(false, 0)
}

func (sc *sshConfig) toJumpsImpl(ignoreIntermediate bool, depth int) ([]Jump, error) {
	if depth > maxJumpRecursions {
		return nil, fmt.Errorf("maximum jump recursions exceeded")
	}

	if err := sc.validate(); err != nil {
		return nil, fmt.Errorf("%v: %v", sc.Alias, err)
	}

	if ignoreIntermediate {
		sc.Jumps = nil
	}

	var s []Jump
	for i, j := range sc.Jumps {
		jc, err := ParseSSHConfig(j.host)
		if err != nil {
			return nil, fmt.Errorf("could not parse SSH config for %v: %v", j.host, err)
		}

		// Replace jump user & port if provided inline
		if j.user != "" {
			jc.User = j.user
		}
		if j.port != 0 {
			jc.Port = j.port
		}
		// If hostname could not be resolved from ssh config, take it literally
		if jc.HostName == "" {
			jc.HostName = j.host
		}

		jc.EnsureUser()

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

	addKeyFiles(sc.IdentityFiles)

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
			return nil, fmt.Errorf("%v: no key files found", sc.Alias)
		}
	}
	log.Debugf("Trying %d key file(s)", len(signers))

	keyCallback, keyAlgos, err := sc.makeCallbackAndAlgos()
	if err != nil {
		return nil, err
	}

	clientConf := &ssh.ClientConfig{
		Config: ssh.Config{
			Ciphers:      sc.Ciphers,
			KeyExchanges: sc.KexAlgos,
			MACs:         sc.Macs,
		},
		User:              sc.User,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signers...)},
		HostKeyAlgorithms: keyAlgos,
		HostKeyCallback:   keyCallback,
		Timeout:           sshConnTimeout,
	}

	newJump := Jump{HostName: sc.HostName, Port: sc.Port, ClientConfig: clientConf}
	s = append(s, newJump)

	return s, nil
}

func (sc *sshConfig) makeCallbackAndAlgos() (cb ssh.HostKeyCallback, algs []string, err error) {
	if sc.KeyCheck == strict {
		var hosts []string
		for _, k := range sc.KnownHostsFiles {
			k = paths.ReplaceTilde(k)
			if _, err := os.Stat(k); err != nil {
				log.Debugf("could not open known hosts file %v: %v", k, err)
				continue
			}
			hosts = append(hosts, k)
		}
		if cb, err = knownhosts.New(hosts...); err != nil {
			return nil, nil, fmt.Errorf("knownhosts: %v", err)
		}
		known := extractHostKeyAlgos(cb, net.JoinHostPort(sc.HostName, strconv.Itoa(sc.Port)))
		algs = filter(sc.HostKeyAlgos, known)
		if len(algs) == 0 {
			return nil, nil, fmt.Errorf("%v: no suitable host key algorithms found: configured are %v, "+
				"available in known_hosts are %v. %v%vNote that boring does not automatically add keys to "+
				"your known_hosts.%v", sc.Alias, sc.HostKeyAlgos, known, log.Bold, log.Red, log.Reset)
		}
		log.Debugf("%v: key types in known_hosts: %v, configured: %v, trying: %v",
			sc.Alias, known, sc.HostKeyAlgos, algs)
	} else if sc.KeyCheck == off {
		cb = ssh.InsecureIgnoreHostKey()
		algs = sc.HostKeyAlgos
	}
	return
}

func (sc *sshConfig) validate() error {
	if sc.HostName == "" {
		return fmt.Errorf("no host specified.")
	}
	if sc.User == "" {
		return fmt.Errorf("no user specified.")
	}
	if sc.Port == 0 {
		return fmt.Errorf("no port specified.")
	}
	return nil
}

func (sc *sshConfig) EnsureUser() {
	// Like ssh(1), use $USER if no user specified
	if sc.User == "" {
		if u, err := user.Current(); err == nil {
			sc.User = u.Username
		}
	}
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

func split(s string) []string {
	return strings.Split(s, ",")
}

func filter(alist, allowed []string) []string {
	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		set[a] = struct{}{}
	}

	var out []string
	for _, a := range alist {
		if _, ok := set[a]; ok {
			out = append(out, a)
		}
	}
	return out
}
