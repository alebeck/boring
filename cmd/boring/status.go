package main

import (
	"fmt"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

func status(t *tunnel.TunnelDesc) string {
	switch t.Status {
	case tunnel.Closed:
		return log.Red + "closed" + log.Reset
	case tunnel.Reconn:
		return log.Yellow + "reconn" + log.Reset
	}

	// Tunnel is open, show uptime
	since := time.Since(t.LastConn)
	days := int(since / (24 * time.Hour))
	hours := int(since/time.Hour) % 24
	mins := int(since/time.Minute) % 60
	secs := int(since/time.Second) % 60
	var str string
	if days > 0 {
		str = fmt.Sprintf("%02dd%02dh", days, hours)
	} else if hours > 0 {
		str = fmt.Sprintf("%02dh%02dm", hours, mins)
	} else {
		str = fmt.Sprintf("%02dm%02ds", mins, secs)
	}
	return log.Bold + log.Green + str + log.Reset
}
