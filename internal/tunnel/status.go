package tunnel

import (
	"fmt"

	"github.com/alebeck/boring/internal/log"
)

type Status int

const (
	Closed Status = iota
	Open
	Reconn
)

var statusNames = map[Status]string{
	Closed: log.ColorRed + "CLOSED" + log.ColorReset,
	Open:   log.ColorGreen + "OPEN" + log.ColorReset,
	Reconn: log.ColorYellow + "RECONN" + log.ColorReset,
}

func (s Status) String() string {
	n, ok := statusNames[s]
	if !ok {
		return fmt.Sprintf("%d", int(s))
	}
	return n
}
