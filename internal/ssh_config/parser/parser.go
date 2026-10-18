package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/alebeck/boring/internal/paths"
)

const maxIncludeDepth = 5

// errDepthExceeded is returned if too many Include directives are nested,
// which usually indicates an Include loop
var errDepthExceeded = errors.New("ssh_config: max recurse depth exceeded")

// TODO extend this by at least "localnetwork"
var allowedMatchKeywords = []string{"host", "originalhost", "user", "localuser"}

type sshParser struct {
	flow   chan token
	blocks config
	// system is set for files in /etc/ssh, against which relative Include
	// paths are resolved
	system bool
	depth  uint8
}

// parseFile parses the config file at path, depth is its Include nesting level
func parseFile(path string, depth uint8) (c config, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// The parser reports errors by panicking
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()
	system := strings.HasPrefix(filepath.Clean(path), "/etc/ssh")
	return parseSSH(lexSSH(b), system, depth), nil
}

func parseSSH(flow chan token, system bool, depth uint8) config {
	// Ensure we consume tokens to completion even if parser exits early
	defer func() {
		for range flow {
		}
	}()

	p := &sshParser{
		flow: flow,
		// The file starts with an implicit "Host *" block
		blocks: config{{matcher: hostPatterns{newPattern("*")}}},
		system: system,
		depth:  depth,
	}
	p.run()
	return p.blocks
}

func (p *sshParser) raiseErrorf(tok *token, format string, args ...any) {
	panic(fmt.Errorf("%v: %s", tok.position, fmt.Sprintf(format, args...)))
}

func (p *sshParser) getToken() *token {
	tok, ok := <-p.flow
	if !ok {
		return nil
	}
	return &tok
}

func (p *sshParser) run() {
	for tok := p.getToken(); tok != nil; tok = p.getToken() {
		switch tok.typ {
		case tokenComment, tokenEmptyLine:
		case tokenKey:
			p.parseKV(tok)
		case tokenEOF:
			return
		default:
			p.raiseErrorf(tok, "unexpected token %q", tok.val)
		}
	}
}

func (p *sshParser) parseKV(key *token) {
	val := p.getToken()
	if val.typ == tokenEquals {
		val = p.getToken()
	}
	switch lkey := strings.ToLower(key.val); lkey {
	case "host":
		var patterns hostPatterns
		for s := range strings.FieldsSeq(val.val) {
			patterns = append(patterns, newPattern(s))
		}
		p.blocks = append(p.blocks, &block{matcher: patterns})
	case "match":
		p.blocks = append(p.blocks, p.parseMatch(val))
	case "include":
		p.addNode(p.parseInclude(val))
	default:
		v := strings.TrimRightFunc(val.val, unicode.IsSpace)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		p.addNode(kv{key: lkey, value: v})
	}
}

func (p *sshParser) addNode(n any) {
	last := p.blocks[len(p.blocks)-1]
	last.nodes = append(last.nodes, n)
}

func (p *sshParser) parseMatch(val *token) *block {
	b := &block{}
	criteria := matchCriteria{}
	args := strings.Fields(strings.ToLower(val.val))
	for i := 0; i < len(args); i++ {
		switch k := args[i]; k {
		case "canonical":
			p.raiseErrorf(val, "'canonical' keyword not supported")
		case "final":
			b.final = true
		case "all":
			alone := len(args) == 1
			afterFinal := len(args) == 2 && i == 1 && args[0] == "final"
			if !alone && !afterFinal {
				p.raiseErrorf(val, "'all' keyword must be alone or immediately after 'final'")
			}
		default:
			if !slices.Contains(allowedMatchKeywords, k) {
				p.raiseErrorf(val, "Match keyword not supported: %v", k)
			}
			i++
			if i >= len(args) {
				p.raiseErrorf(val, "No value found after Match keyword %q", k)
			}
			criteria[k] = newPattern(args[i])
		}
	}
	b.matcher = criteria
	return b
}

func (p *sshParser) parseInclude(val *token) include {
	depth := p.depth + 1
	if depth > maxIncludeDepth {
		panic(errDepthExceeded)
	}
	var inc include
	seen := map[string]bool{}
	for d := range strings.FieldsSeq(val.val) {
		matches, err := filepath.Glob(p.includePath(d))
		if err != nil {
			p.raiseErrorf(val, "Error parsing Include directive: %v", err)
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			seen[m] = true
			c, err := parseFile(m, depth)
			if errors.Is(err, errDepthExceeded) {
				panic(err)
			}
			if err != nil {
				p.raiseErrorf(val, "Error parsing Include directive: %v", err)
			}
			inc = append(inc, c)
		}
	}
	return inc
}

func (p *sshParser) includePath(path string) string {
	switch {
	case filepath.IsAbs(path):
		return path
	case p.system:
		return filepath.Join("/etc/ssh", path)
	case strings.HasPrefix(path, "~/"):
		return paths.ReplaceTilde(path)
	default:
		return filepath.Join(paths.ReplaceTilde("~"), ".ssh", path)
	}
}
