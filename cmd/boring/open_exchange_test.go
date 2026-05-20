package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
)

func TestMain(m *testing.M) {
	log.Init(io.Discard, false, false)
	os.Exit(m.Run())
}

func TestRunOpenExchangeWithPrompt(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		br := bufio.NewReader(server)
		var cmd daemon.Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		_ = daemon.WriteMsg(server, daemon.MsgAuthPrompt, daemon.AuthPrompt{
			Name: "2fa", Questions: []string{"Code:"}, Echo: []bool{false},
		})
		env, err := daemon.ReadEnvelope(br)
		if err != nil || env.Type != daemon.MsgAuthReply {
			return
		}
		_ = daemon.WriteMsg(server, daemon.MsgResp, daemon.Resp{Success: true})
	}()

	var gotQ []string
	prompter := auth.FuncPrompter(func(_, _ string, qs []string, _ []bool) ([]string, error) {
		gotQ = qs
		return []string{"123456"}, nil
	})
	resp, err := runOpenExchange(client, daemon.Cmd{Kind: daemon.Open}, prompter)
	if err != nil {
		t.Fatalf("runOpenExchange: %v", err)
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
		var cmd daemon.Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		_ = daemon.WriteMsg(server, daemon.MsgResp, daemon.Resp{Success: true})
	}()

	resp, err := runOpenExchange(client, daemon.Cmd{Kind: daemon.Open}, nil)
	if err != nil {
		t.Fatalf("runOpenExchange: %v", err)
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
		var cmd daemon.Cmd
		if err := ipc.Read(&cmd, br); err != nil {
			return
		}
		// MsgAuthReply is a type the client never expects FROM the daemon.
		_ = daemon.WriteMsg(server, daemon.MsgAuthReply, daemon.AuthReply{})
	}()

	if _, err := runOpenExchange(client, daemon.Cmd{Kind: daemon.Open}, nil); err == nil {
		t.Fatal("expected an error for an unexpected message type")
	}
}

func TestSyncPrompterDelegates(t *testing.T) {
	p := &syncPrompter{inner: auth.FuncPrompter(
		func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return []string{"ok"}, nil
		})}
	ans, err := p.Prompt("n", "i", []string{"q"}, []bool{true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ans) != 1 || ans[0] != "ok" {
		t.Fatalf("got %v, want [ok]", ans)
	}
}
