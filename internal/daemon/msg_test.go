package daemon

import (
	"bufio"
	"io"
	"net"
	"os"
	"reflect"
	"testing"

	"github.com/alebeck/boring/internal/log"
)

func TestMain(m *testing.M) {
	log.Init(io.Discard, false, false)
	os.Exit(m.Run())
}

func TestEnvelopeRoundTrip(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	want := AuthPrompt{
		Name:        "2fa",
		Instruction: "Enter code",
		Questions:   []string{"Code:"},
		Echo:        []bool{false},
	}
	writeErr := make(chan error, 1)
	go func() { writeErr <- writeMsg(b, MsgAuthPrompt, want) }()

	br := bufio.NewReader(a)
	env, err := readEnvelope(br)
	if err != nil {
		t.Fatalf("readEnvelope: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writeMsg: %v", err)
	}
	if env.Type != MsgAuthPrompt {
		t.Fatalf("type = %q, want %q", env.Type, MsgAuthPrompt)
	}
	got, err := decodeAuthPrompt(env)
	if err != nil {
		t.Fatalf("decodeAuthPrompt: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEnvelopeRoundTripAuthReply(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	want := AuthReply{Answers: []string{"123456"}}
	writeErr := make(chan error, 1)
	go func() { writeErr <- writeMsg(b, MsgAuthReply, want) }()

	br := bufio.NewReader(a)
	env, err := readEnvelope(br)
	if err != nil {
		t.Fatalf("readEnvelope: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writeMsg: %v", err)
	}
	if env.Type != MsgAuthReply {
		t.Fatalf("type = %q, want %q", env.Type, MsgAuthReply)
	}
	got, err := decodeAuthReply(env)
	if err != nil {
		t.Fatalf("decodeAuthReply: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDecodeAuthPromptMalformed(t *testing.T) {
	env := envelope{Type: MsgAuthPrompt, Payload: []byte(`{"questions":"not-an-array"}`)}
	if _, err := decodeAuthPrompt(env); err == nil {
		t.Fatal("expected error decoding malformed payload")
	}
}
