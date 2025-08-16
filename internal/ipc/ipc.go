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
