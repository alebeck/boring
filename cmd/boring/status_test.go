package main

import (
	"io"
	"testing"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

func TestStatusClosed(t *testing.T) {
	d := &tunnel.Desc{Status: tunnel.Closed}
	if s := status(d); s != "closed" {
		t.Fatalf("incorrect status: %s", s)
	}
}

func TestStatusReconn(t *testing.T) {
	d := &tunnel.Desc{Status: tunnel.Reconn}
	if s := status(d); s != "reconn" {
		t.Fatalf("incorrect status: %s", s)
	}
}

func TestStatusUptimeMins(t *testing.T) {
	log.Init(io.Discard, true, false)
	l := 7*time.Minute + 21*time.Second
	d := &tunnel.Desc{
		Status:   tunnel.Open,
		LastConn: time.Now().Add(-l),
	}
	if s := status(d); s != "07m21s" {
		t.Fatalf("incorrect uptime: %s", s)
	}
}

func TestStatusUptimeHours(t *testing.T) {
	log.Init(io.Discard, true, false)
	l := 3*time.Hour + 42*time.Minute
	d := &tunnel.Desc{
		Status:   tunnel.Open,
		LastConn: time.Now().Add(-l),
	}
	if s := status(d); s != "03h42m" {
		t.Fatalf("incorrect uptime: %s", s)
	}
}

func TestStatusUptimeDays(t *testing.T) {
	log.Init(io.Discard, true, false)
	l := 2*24*time.Hour + 3*time.Hour
	d := &tunnel.Desc{
		Status:   tunnel.Open,
		LastConn: time.Now().Add(-l),
	}
	if s := status(d); s != "02d03h" {
		t.Fatalf("incorrect uptime: %s", s)
	}
}
