package ssh_config

import (
	"fmt"
	"strconv"
	"strings"
)

// jumpSpec represents an ssh_config ProxyJump entry
type jumpSpec struct {
	host string
	user string
	port int
}

func parseProxyJump(s string) (*jumpSpec, error) {
	// Format: [user@]host[:port]
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
