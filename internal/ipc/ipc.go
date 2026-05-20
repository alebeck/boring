package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alebeck/boring/internal/log"
)

func Write(s any, w io.Writer) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to serialize: %v", err)
	}
	log.Debugf("Sending: %v", string(data))

	_, err = w.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write: %v", err)
	}
	return nil
}

// Read decodes one newline-delimited JSON message into s. To read several
// messages from one connection, pass the SAME *bufio.Reader to every call;
// a raw net.Conn would lose bytes buffered past the first newline.
func Read(s any, r io.Reader) error {
	br := bufio.NewReader(r)
	data, err := br.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read: %w", err)
	}
	log.Debugf("Received: %v", string(data))

	err = json.Unmarshal(data, s)
	if err != nil {
		return fmt.Errorf("failed to deserialize: %w", err)
	}
	return nil
}
