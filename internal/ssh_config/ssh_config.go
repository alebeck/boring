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
	ossh_config "github.com/alebeck/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	sshConnTimeout    = 10 * time.Second
	maxJumpRecursions = 20
)

var overrideConfig = os.Getenv("BORING_SSH_CONFIG")

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

// Hop holds information needed to establish a single SSH hop
type Hop struct {
	HostName string
	Port     int
	*ssh.ClientConfig
}

// SSHConfig represents an SSH config read from, e.g., ~/.ssh/config
type SSHConfig struct {
	Alias            string
	User             string
	HostName         string
	Port             int
	KeyCheck         keyCheck
	IdentitiesOnly   bool
	IdentityFiles    []string
	CertificateFiles []string
	KnownHostsFiles  []string
	Ciphers          []string
	Macs             []string
	HostKeyAlgos     []string
	KexAlgos         []string
	Jumps            []*jumpSpec
}

var (
	hostnameTokens  = []string{"%%", "%h"}
	proxyTokens     = []string{"%%", "h", "%n", "%p", "%r"}
	identFileTokens = []string{
		"%%", "%d", "%h", "%i", "%j", "%k",
		"%L", "%l", "%n", "%p", "%r", "%u",
	}
)

func ParseSSHConfig(alias, user string) (*SSHConfig, error) {
	// We create a new ssh_config.UserSettings object at each connection so that
	// config file changes are reflected immediately.
	us := ossh_config.MakeDefaultUserSettings()
	if overrideConfig != "" {
		us.ConfigFinder(func() string { return overrideConfig })
	}

	// This is a "strict" dummy query to catch potential parsing errors early
	if _, err := us.GetStrict(alias, "HostName", ""); err != nil {
		return nil, err
	}

	// In the following, we always provide `user` since it is needed for `Match` matching
	get := func(key string) string { return us.Get(alias, key, user) }
	getAll := func(key string) []string { return us.GetAll(alias, key, user) }

	c := &SSHConfig{Alias: alias}
	sub := makeSubst(alias)

	if c.HostName = sub.apply(get("HostName"), hostnameTokens); c.HostName != "" {
		sub["%h"] = c.HostName
	}

	c.User = get("User")
	sub["%r"] = c.User
	c.Port, _ = strconv.Atoi(get("Port"))
	sub["%p"] = fmt.Sprintf("%d", c.Port)

	s := get("StrictHostKeyChecking")
	if s == "no" || s == "off" {
		c.KeyCheck = off
	} else if s == "accept-new" {
		log.Warningf(
			"StrictHostKeyChecking 'accept-new' not supported, using 'yes'")
	} else if s != "yes" && s != "ask" {
		return nil, fmt.Errorf(
			"unsupported StrictHostKeyChecking option '%v'", s)
	}

	c.Ciphers = split(get("Ciphers"))
	c.Macs = split(get("MACs"))
	c.HostKeyAlgos = split(get("HostKeyAlgorithms"))
	c.KexAlgos = split(get("KexAlgorithms"))

	// Jump hosts
	pj := sub.apply(get("ProxyJump"), proxyTokens)
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

	c.IdentitiesOnly = get("IdentitiesOnly") == "yes"
	c.IdentityFiles = sub.applyAll(getAll("IdentityFile"), identFileTokens)
	c.CertificateFiles = getAll("CertificateFile")

	// Known hosts
	hosts := getAll("GlobalKnownHostsFile")
	hosts = append(hosts, sub.applyAll(getAll("UserKnownHostsFile"), identFileTokens)...)
	for _, h := range hosts {
		c.KnownHostsFiles = append(c.KnownHostsFiles, strings.Split(h, " ")...)
	}

	return c, nil
}

// ToHops creates an ordered series of Hops from an SSHConfig
func (sc *SSHConfig) ToHops() ([]Hop, error) {
	return sc.toHopsImpl(false, 0)
}

func (sc *SSHConfig) toHopsImpl(ignoreIntermediate bool, depth int) ([]Hop, error) {
	if depth > maxJumpRecursions {
		return nil, fmt.Errorf("maximum jump recursions exceeded")
	}

	if err := sc.validate(); err != nil {
		return nil, fmt.Errorf("%v: %v", sc.Alias, err)
	}

	if ignoreIntermediate {
		sc.Jumps = nil
	}

	var hops []Hop
	for i, j := range sc.Jumps {
		jc, err := ParseSSHConfig(j.host, j.user)
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
		hs, err := jc.toHopsImpl(i != 0, depth+1)
		if err != nil {
			return nil, err
		}
		hops = append(hops, hs...)
	}

	sigs, err := sc.makeSigners()
	if err != nil {
		return nil, err
	}
	log.Debugf("Trying %d key file(s)", len(sigs))
	auth := []ssh.AuthMethod{ssh.PublicKeys(sigs...)}

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
		Auth:              auth,
		HostKeyAlgorithms: keyAlgos,
		HostKeyCallback:   keyCallback,
		Timeout:           sshConnTimeout,
	}

	hop := Hop{HostName: sc.HostName, Port: sc.Port, ClientConfig: clientConf}
	hops = append(hops, hop)

	return hops, nil
}

func (sc *SSHConfig) loadCerts() (certs []*ssh.Certificate) {
	for _, f := range sc.CertificateFiles {
		cert, err := loadCert(f)
		if err != nil {
			log.Warningf("Certificate file %q could not be added: %v", f, err)
			continue
		}
		log.Debugf("Loaded certificate: %s", f)
		certs = append(certs, cert)
	}
	return
}

type identity struct {
	signer ssh.Signer
	path   string // non-empty only for IdentityFiles
}

func (sc *SSHConfig) loadIDs() (fileIDs, agentCertIDs, agentCfgIDs, agentOtherIDs []identity) {
	cfgFP := make(map[string]struct{}, len(sc.IdentityFiles))

	for _, f := range sc.IdentityFiles {
		s, err := loadPrivateKey(f)
		if err != nil {
			log.Warningf("key file %q could not be added: %v", f, err)
			// Here we still try to load the corresponding public key to mark it configured
			if pub, err := loadPublicKey(f + ".pub"); err == nil {
				// If .pub is a certificate, fingerprint its underlying key.
				if c, ok := pub.(*ssh.Certificate); ok {
					cfgFP[keyFP(c.Key)] = struct{}{}
				} else {
					cfgFP[keyFP(pub)] = struct{}{}
				}
			}
			continue
		}
		fileIDs = append(fileIDs, identity{signer: s, path: f})
		cfgFP[keyFP(s.PublicKey())] = struct{}{}
	}

	if agSigs, err := agent.GetSigners(); err != nil {
		log.Warningf("Unable to get keys from ssh-agent: %v", err)
	} else {
		for _, s := range agSigs {
			// Agent may return certificate identities (public key is a cert)
			if c, ok := s.PublicKey().(*ssh.Certificate); ok {
				fp := keyFP(c.Key)
				if _, ok := cfgFP[fp]; ok || !sc.IdentitiesOnly {
					agentCertIDs = append(agentCertIDs, identity{signer: s})
				}
				continue
			}

			id := identity{signer: s}
			fp := keyFP(s.PublicKey())
			if _, ok := cfgFP[fp]; ok {
				agentCfgIDs = append(agentCfgIDs, id)
				// Remove id from fileIDs if existing
				for i, fid := range fileIDs {
					if keyFP(fid.signer.PublicKey()) == fp {
						fileIDs = append(fileIDs[:i], fileIDs[i+1:]...)
						break
					}
				}
			} else if !sc.IdentitiesOnly {
				agentOtherIDs = append(agentOtherIDs, id)
			}
		}
	}
	return
}

func (sc *SSHConfig) makeSigners() ([]ssh.Signer, error) {
	// https://github.com/openssh/openssh-portable/blob/832a77000abe61f61bddb9e595f45c7131c0269d/sshconnect2.c#L1669
	// Order (OpenSSH-like):
	// 1. CertificateFile certs (bound to first matching private key)
	// 2. Agent keys that match IdentityFiles
	// 3. Other agent keys (unless IdentitiesOnly)
	// 4. IdentityFile keys
	// + agent certificate identities (already certified signers)

	// Load ID groups
	fileIDs, agentCertIDs, agentCfgIDs, agentOtherIDs := sc.loadIDs()

	var sigs []ssh.Signer
	idsForCert := append([]identity{}, agentCfgIDs...)
	idsForCert = append(idsForCert, agentOtherIDs...)
	idsForCert = append(idsForCert, fileIDs...)

	bind := func(c *ssh.Certificate) {
		for _, id := range idsForCert {
			if certSig, err := certify(c, id.signer); err == nil {
				sigs = append(sigs, certSig)
				return
			}
		}
	}

	if certs := sc.loadCerts(); len(certs) > 0 {
		// CertificateFile certs: bind each cert to first matching key
		for _, c := range certs {
			bind(c)
		}
	} else {
		// Implicit certs
		for _, path := range sc.IdentityFiles {
			c, err := loadCert(path + "-cert.pub")
			if err != nil {
				continue
			}
			bind(c)
		}
	}

	// Try already-certified agent identities (certificate signers)
	for _, id := range agentCertIDs {
		sigs = append(sigs, id.signer)
	}

	// Plain keys as fallback (bucket order)
	for _, id := range agentCfgIDs {
		sigs = append(sigs, id.signer)
	}
	for _, id := range agentOtherIDs {
		sigs = append(sigs, id.signer)
	}
	for _, id := range fileIDs {
		sigs = append(sigs, id.signer)
	}

	if len(sigs) == 0 {
		return nil, fmt.Errorf("%s: no key files found", sc.Alias)
	}

	sigs = dedupeSigners(sigs)

	for _, sig := range sigs {
		log.Debugf("%s: will try key %s", sc.Alias, sig)
	}

	return sigs, nil
}

func (sc *SSHConfig) makeCallbackAndAlgos() (cb ssh.HostKeyCallback, algs []string, err error) {
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
			return nil, nil, fmt.Errorf("%v: could not determine host key algorithms: default are %v, "+
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

func (sc *SSHConfig) validate() error {
	if sc.HostName == "" {
		return fmt.Errorf("no host specified")
	}
	if sc.User == "" {
		return fmt.Errorf("no user specified")
	}
	if sc.Port == 0 {
		return fmt.Errorf("no port specified")
	}
	return nil
}

func (sc *SSHConfig) EnsureUser() {
	// Like ssh(1), use $USER if no user specified
	if sc.User == "" {
		if u, err := user.Current(); err == nil {
			sc.User = u.Username
		}
	}
}

func loadPrivateKey(path string) (ssh.Signer, error) {
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
	return signer, nil
}

func loadPublicKey(path string) (ssh.PublicKey, error) {
	if path == "" {
		return nil, fmt.Errorf("no key specified")
	}
	raw, err := os.ReadFile(paths.ReplaceTilde(path))
	if err != nil {
		return nil, fmt.Errorf("could not read public key file: %v", err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return nil, fmt.Errorf("could not parse public key file: %v", err)
	}
	return pub, nil
}

func loadCert(path string) (*ssh.Certificate, error) {
	raw, err := os.ReadFile(paths.ReplaceTilde(path))
	if err != nil {
		return nil, fmt.Errorf("could not read certificate file: %v", err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return nil, fmt.Errorf("could not parse certificate file: %v", err)
	}
	cert, ok := pub.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("file does not contain a valid SSH certificate")
	}
	return cert, nil
}

func certify(cert *ssh.Certificate, sig ssh.Signer) (ssh.Signer, error) {
	if _, ok := sig.PublicKey().(*ssh.Certificate); ok {
		return nil, fmt.Errorf("signer is already a certificate identity")
	}
	if keyFP(sig.PublicKey()) != keyFP(cert.Key) {
		return nil, fmt.Errorf("signer does not match certificate key")
	}
	certSig, err := ssh.NewCertSigner(cert, sig)
	if err != nil {
		return nil, fmt.Errorf("could not create certified signer: %v", err)
	}
	return certSig, nil
}

func dedupeSigners(sigs []ssh.Signer) []ssh.Signer {
	seen := make(map[string]struct{}, len(sigs))
	out := make([]ssh.Signer, 0, len(sigs))
	for _, s := range sigs {
		fp := keyFP(s.PublicKey())
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		out = append(out, s)
	}
	return out
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

// keyFP returns a fingerprint string for a public key
// we can make this more sophisticated later if needed
func keyFP(k ssh.PublicKey) string {
	return string(k.Marshal())
}
