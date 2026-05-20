package daemon

import (
	"bufio"
	"net"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/ipc"
)

func TestRunOpenExchangeWithPrompt(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		br := bufio.NewReader(server)
		var cmd Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		_ = WriteMsg(server, MsgAuthPrompt, AuthPrompt{
			Name: "2fa", Questions: []string{"Code:"}, Echo: []bool{false},
		})
		env, err := ReadEnvelope(br)
		if err != nil || env.Type != MsgAuthReply {
			return
		}
		_ = WriteMsg(server, MsgResp, Resp{Success: true})
	}()

	var gotQ []string
	prompter := auth.FuncPrompter(func(_, _ string, qs []string, _ []bool) ([]string, error) {
		gotQ = qs
		return []string{"123456"}, nil
	})
	resp, err := RunOpenExchange(client, Cmd{Kind: Open}, prompter)
	if err != nil {
		t.Fatalf("RunOpenExchange: %v", err)
	}
	if !resp.Success {
		t.Fatalf("resp not success: %+v", resp)
	}
	if len(gotQ) != 1 || gotQ[0] != "Code:" {
		t.Fatalf("prompt questions not delivered: %v", gotQ)
	}
}

func TestRunOpenExchangeNoPrompt(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		br := bufio.NewReader(server)
		var cmd Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		_ = WriteMsg(server, MsgResp, Resp{Success: true})
	}()

	resp, err := RunOpenExchange(client, Cmd{Kind: Open}, nil)
	if err != nil {
		t.Fatalf("RunOpenExchange: %v", err)
	}
	if !resp.Success {
		t.Fatalf("resp not success: %+v", resp)
	}
}

func TestRunOpenExchangeUnexpectedMessage(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		br := bufio.NewReader(server)
		var cmd Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		// MsgAuthReply is a type the client never expects FROM the daemon.
		_ = WriteMsg(server, MsgAuthReply, AuthReply{})
	}()

	if _, err := RunOpenExchange(client, Cmd{Kind: Open}, nil); err == nil {
		t.Fatal("expected an error for an unexpected message type")
	}
}
