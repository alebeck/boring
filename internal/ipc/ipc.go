package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"github.com/alebeck/boring/internal/log"
)

func Send(s any, conn net.Conn) error {
	log.Debugf("Sending: %v", s)

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to serialize response: %v", err)
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to send response: %v", err)
	}
	return nil
}

func Receive(s any, conn net.Conn) error {
	reader := bufio.NewReader(conn)
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read from connection: %v", err)
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		return fmt.Errorf("failed to deserialize command: %v", err)
	}
	log.Debugf("Received object: %v", s)
	return nil
}
