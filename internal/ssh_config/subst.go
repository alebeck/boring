package ssh_config

import (
	"os"
	"os/user"
	"strings"
)

type subst map[string]string

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
