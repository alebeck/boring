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
	Closed: log.ColorRed + "Closed" + log.ColorReset,
	Open:   log.ColorGreen + "Open" + log.ColorReset,
	Reconn: log.ColorYellow + "Reconn" + log.ColorReset,
}

func (s Status) String() string {
	n, ok := statusNames[s]
	if !ok {
		return fmt.Sprintf("%d", int(s))
	}
	return n
}
