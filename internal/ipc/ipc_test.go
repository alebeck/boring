package ipc

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alebeck/boring/internal/log"
)

func TestMain(m *testing.M) {
	log.Init(os.Stdout, true, false)
	os.Exit(m.Run())
}

func TestIPC(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Avoid test hangs if something goes wrong
	c1.SetDeadline(time.Now().Add(1 * time.Second))
	c2.SetDeadline(time.Now().Add(1 * time.Second))

	done := make(chan struct{})
	obj := map[string]string{"foo": "bar"}

	go func() {
		defer close(done)
		var r map[string]string
		if err := Read(&r, c2); err != nil {
			t.Errorf("receive failed: %v", err)
		}
		if !reflect.DeepEqual(obj, r) {
			t.Errorf("wrong data: %v != %v", obj, r)
		}
	}()

	if err := Write(obj, c1); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	<-done
}

func TestWriteError(t *testing.T) {
	var obj int
	c1, c2 := net.Pipe()
	defer c1.Close()
	c2.Close() // peer closed
	if err := Write(obj, c1); err == nil ||
		!strings.Contains(err.Error(), "failed to write") {
		t.Fatalf("did not get expected error")
	}
}

func TestReadError(t *testing.T) {
	var obj map[string]string
	c1, c2 := net.Pipe()
	defer c1.Close()
	c2.Close() // peer closed
	if err := Read(&obj, c1); err == nil ||
		!strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("did not get expected error")
	}
}

func TestSerializeError(t *testing.T) {
	var v = make(chan int) // not serializable
	if err := Write(v, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "failed to serialize") {
		t.Fatalf("did not get expected error")
	}
}

func TestDeserializeError(t *testing.T) {
	var v = make(chan int) // not serializable
	if err := Read(&v, strings.NewReader("test\n")); err == nil ||
		!strings.Contains(err.Error(), "failed to deserialize") {
		t.Fatalf("did not get expected error")
	}
}

func TestReadMultipleMessagesSharedReader(t *testing.T) {
	// A bytes.Buffer holds both messages before the first read, so the
	// first Read buffers message 2's bytes inside the *bufio.Reader.
	// The second Read finds them only because the same reader is reused;
	// a fresh bufio.Reader on the raw buffer would have lost them.
	var transport bytes.Buffer
	if err := Write(map[string]string{"a": "1"}, &transport); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if err := Write(map[string]string{"a": "2"}, &transport); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	br := bufio.NewReader(&transport)
	var first, second map[string]string
	if err := Read(&first, br); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if err := Read(&second, br); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if first["a"] != "1" || second["a"] != "2" {
		t.Fatalf("got %v, %v", first, second)
	}
}
