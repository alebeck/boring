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
	Closed: log.Red + "closed" + log.Reset,
	Open:   log.Green + "open" + log.Reset,
	Reconn: log.Yellow + "reconn" + log.Reset,
}

func (s Status) String() string {
	n, ok := statusNames[s]
	if !ok {
		return fmt.Sprintf("%d", int(s))
	}
	return n
}
