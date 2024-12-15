package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"github.com/alebeck/boring/internal/log"
)

func Send(s any, conn net.Conn) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to serialize: %v", err)
	}
	log.Debugf("Sending: %v", string(data))

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to send: %v", err)
	}
	return nil
}

func Receive(s any, conn net.Conn) error {
	reader := bufio.NewReader(conn)
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read from connection: %w", err)
	}
	log.Debugf("Received: %v", string(data))

	err = json.Unmarshal(data, s)
	if err != nil {
		return fmt.Errorf("failed to deserialize: %w", err)
	}
	return nil
}
