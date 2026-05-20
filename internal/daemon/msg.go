package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alebeck/boring/internal/ipc"
)

// MsgType tags a message so a single IPC stream can multiplex several kinds.
type MsgType string

const (
	MsgResp       MsgType = "resp"
	MsgAuthPrompt MsgType = "auth_prompt"
	MsgAuthReply  MsgType = "auth_reply"
)

// AuthPrompt is sent by the daemon to request interactive auth input.
// Its shape mirrors an SSH keyboard-interactive challenge.
type AuthPrompt struct {
	Name        string   `json:"name"`
	Instruction string   `json:"instruction"`
	Questions   []string `json:"questions"`
	Echo        []bool   `json:"echo"`
}

// AuthReply carries the client's answers to an AuthPrompt.
type AuthReply struct {
	Answers []string `json:"answers"`
	// Err is set when the user or client aborted the prompt.
	Err string `json:"err,omitempty"`
}

// envelope wraps any message with its type for multiplexed streams.
type envelope struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// writeMsg serializes v and writes a typed envelope to w.
func writeMsg(w io.Writer, t MsgType, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", t, err)
	}
	return ipc.Write(envelope{Type: t, Payload: payload}, w)
}

// readEnvelope reads one typed envelope from r. The reader must be a shared
// *bufio.Reader so bytes buffered past the first message survive across calls.
func readEnvelope(r *bufio.Reader) (envelope, error) {
	var e envelope
	if err := ipc.Read(&e, r); err != nil {
		return e, fmt.Errorf("failed to read envelope: %w", err)
	}
	return e, nil
}

func decodeAuthPrompt(e envelope) (AuthPrompt, error) {
	var p AuthPrompt
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return p, fmt.Errorf("failed to decode %s: %w", MsgAuthPrompt, err)
	}
	return p, nil
}

func decodeAuthReply(e envelope) (AuthReply, error) {
	var r AuthReply
	if err := json.Unmarshal(e.Payload, &r); err != nil {
		return r, fmt.Errorf("failed to decode %s: %w", MsgAuthReply, err)
	}
	return r, nil
}

func decodeResp(e envelope) (Resp, error) {
	var r Resp
	if err := json.Unmarshal(e.Payload, &r); err != nil {
		return r, fmt.Errorf("failed to decode %s: %w", MsgResp, err)
	}
	return r, nil
}
