package ipc

import (
	"github.com/alebeck/boring/internal/log"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
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
		if err := Receive(&r, c2); err != nil {
			t.Errorf("receive failed: %v", err)
		}
		if !reflect.DeepEqual(obj, r) {
			t.Errorf("wrong data: %v != %v", obj, r)
		}
	}()

	if err := Send(obj, c1); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	<-done
}

func TestSendError(t *testing.T) {
	obj := map[string]string{"foo": "bar"}
	c1, c2 := net.Pipe()
	defer c1.Close()
	c2.Close() // peer closed
	if err := Send(obj, c1); err == nil {
		t.Fatalf("did not get expected error")
	}
}

func TestReceiveError(t *testing.T) {
	var obj map[string]string
	c1, c2 := net.Pipe()
	defer c1.Close()
	c2.Close() // peer closed
	if err := Receive(&obj, c1); err == nil {
		t.Fatalf("did not get expected error")
	}
}
