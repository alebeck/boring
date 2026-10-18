// Package parser reads OpenSSH client config files and resolves the values
// that apply to a host. It is adapted from github.com/kevinburke/ssh_config.
package parser

import (
	"errors"
	"io/fs"
	osuser "os/user"
	"regexp"
	"slices"
	"strings"

	"github.com/alebeck/boring/internal/paths"
)

// modifiableKeys may be prefixed with "+", "-" or "^" to modify their defaults
var modifiableKeys = []string{"ciphers", "macs", "hostkeyalgorithms", "kexalgorithms"}

// Settings holds parsed SSH config files in order of precedence
type Settings struct {
	configs []config
}

// Load parses the SSH config file at path. If path is empty, ~/.ssh/config
// and /etc/ssh/ssh_config are parsed instead, skipping those that don't exist.
func Load(path string) (*Settings, error) {
	files := []string{path}
	if path == "" {
		files = []string{paths.ReplaceTilde("~/.ssh/config"), "/etc/ssh/ssh_config"}
	}
	s := &Settings{}
	for _, f := range files {
		c, err := parseFile(f, 0)
		if path == "" && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		s.configs = append(s.configs, c)
	}
	return s, nil
}

// Get returns the first value of key that applies to the given alias and
// user, or the OpenSSH default if there is none. Keys are case-insensitive.
func (s *Settings) Get(alias, key, user string) string {
	key = strings.ToLower(key)
	ctx := newMatchContext(alias, user)
	for _, c := range s.configs {
		if val := c.get(key, ctx); val != "" {
			if slices.Contains(modifiableKeys, key) {
				val = handleModifiers(val, key)
			}
			return val
		}
	}
	// No value found until now, so check final blocks
	if val := ctx.matchFinal(key); val != "" {
		return val
	}
	return defaults[key]
}

// GetAll returns all values of key that apply to the given alias and user,
// taken from the first config file that sets key, or the OpenSSH default if
// there is none. Keys are case-insensitive.
func (s *Settings) GetAll(alias, key, user string) []string {
	key = strings.ToLower(key)
	ctx := newMatchContext(alias, user)
	for _, c := range s.configs {
		if vals := c.getAll(key, ctx); vals != nil {
			return vals
		}
	}
	// No value found until now, so check final blocks
	if vals := ctx.matchFinalAll(key); vals != nil {
		return vals
	}
	if def := defaults[key]; def != "" {
		return []string{def}
	}
	if key == "identityfile" {
		return defaultIdentities
	}
	return nil
}

// handleModifiers applies a "+", "-" or "^" prefix of val to the default of key
func handleModifiers(val, key string) string {
	if !strings.ContainsRune("+-^", rune(val[0])) {
		return val
	}
	cur := strings.Split(val[1:], ",")
	def := strings.Split(defaults[key], ",")
	var out []string
	switch val[0] {
	case '+':
		out = append(def, cur...)
	case '-':
		for _, a := range def {
			if !slices.Contains(cur, a) {
				out = append(out, a)
			}
		}
	case '^':
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

// config is a parsed config file
type config []*block

func (c config) get(key string, ctx *matchContext) string {
	for _, b := range c {
		if b.final {
			ctx.finalBlocks = append(ctx.finalBlocks, b)
		} else if b.matches(ctx) {
			if val := b.get(key, ctx); val != "" {
				return val
			}
		}
	}
	return ""
}

func (c config) getAll(key string, ctx *matchContext) (vals []string) {
	for _, b := range c {
		if b.final {
			ctx.finalBlocks = append(ctx.finalBlocks, b)
		} else if b.matches(ctx) {
			vals = append(vals, b.getAll(key, ctx)...)
		}
	}
	return vals
}

// block is a Host or Match section and the directives that follow it
type block struct {
	matcher
	final bool  // set for "Match final" blocks
	nodes []any // kv or include
}

// kv is a directive, key is lowercase
type kv struct {
	key, value string
}

// include holds the config files of an Include directive
type include []config

func (b *block) get(key string, ctx *matchContext) string {
	for _, n := range b.nodes {
		switch n := n.(type) {
		case kv:
			if n.key == key {
				return n.value
			}
			ctx.update(n.key, n.value)
		case include:
			for _, c := range n {
				if val := c.get(key, ctx); val != "" {
					return val
				}
			}
		}
	}
	return ""
}

func (b *block) getAll(key string, ctx *matchContext) (vals []string) {
	for _, n := range b.nodes {
		switch n := n.(type) {
		case kv:
			if n.key == key {
				vals = append(vals, n.value)
			}
			ctx.update(n.key, n.value)
		case include:
			for _, c := range n {
				vals = append(vals, c.getAll(key, ctx)...)
			}
		}
	}
	return vals
}

type matcher interface {
	matches(ctx *matchContext) bool
}

// hostPatterns matches if any pattern matches the alias and no negated one does
type hostPatterns []pattern

func (h hostPatterns) matches(ctx *matchContext) bool {
	found := false
	for _, p := range h {
		if p.regex.MatchString(ctx.originalHost) {
			if p.not {
				return false
			}
			found = true
		}
	}
	return found
}

// matchCriteria matches if all criteria match, keyed by Match keyword
type matchCriteria map[string]pattern

func (m matchCriteria) matches(ctx *matchContext) bool {
	for k, p := range m {
		var v string
		switch k {
		case "host":
			v = ctx.host
		case "user":
			v = ctx.user
		case "originalhost":
			v = ctx.originalHost
		case "localuser":
			v = ctx.localUser
		}
		// An empty value never matches
		if v == "" || p.not == p.regex.MatchString(v) {
			return false
		}
	}
	return true
}

// pattern is a Host or Match pattern, see PATTERNS in ssh_config(5)
type pattern struct {
	regex *regexp.Regexp
	not   bool
}

func newPattern(s string) pattern {
	expr := regexp.QuoteMeta(strings.TrimPrefix(s, "!"))
	expr = strings.NewReplacer(`\*`, ".*", `\?`, ".?").Replace(expr)
	return pattern{
		regex: regexp.MustCompile("^" + expr + "$"),
		not:   strings.HasPrefix(s, "!"),
	}
}

// matchContext holds the values that Match criteria are evaluated against
type matchContext struct {
	user         string
	host         string
	localUser    string
	originalHost string
	finalBlocks  []*block
}

func newMatchContext(alias, user string) *matchContext {
	ctx := &matchContext{user: user, host: alias, originalHost: alias}
	if u, err := osuser.Current(); err == nil {
		ctx.localUser = u.Username
	}
	return ctx
}

// update records user and host names set by matching blocks
func (ctx *matchContext) update(key, val string) {
	if ctx == nil {
		return
	}
	switch {
	case key == "user" && ctx.user == "":
		ctx.user = val
	case key == "hostname" && ctx.host == "":
		ctx.host = val
	}
}

func (ctx *matchContext) matchFinal(key string) string {
	for _, b := range ctx.finalBlocks {
		if b.matches(ctx) {
			if val := b.get(key, nil); val != "" {
				return val
			}
		}
	}
	return ""
}

func (ctx *matchContext) matchFinalAll(key string) (vals []string) {
	for _, b := range ctx.finalBlocks {
		if b.matches(ctx) {
			vals = append(vals, b.getAll(key, nil)...)
		}
	}
	return vals
}
