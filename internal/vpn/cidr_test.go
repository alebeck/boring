package vpn

import (
	"net"
	"testing"
)

func TestIPInsideCIDR(t *testing.T) {
	cidrs, err := ParseCIDRs([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	ips := []net.IP{net.ParseIP("10.20.30.40")}
	if !anyIPInCIDRs(ips, cidrs) {
		t.Fatalf("IP did not match CIDR")
	}
}

func TestIPOutsideCIDR(t *testing.T) {
	cidrs, err := ParseCIDRs([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	ips := []net.IP{net.ParseIP("192.168.1.10")}
	if anyIPInCIDRs(ips, cidrs) {
		t.Fatalf("IP matched CIDR")
	}
}

func TestMultipleCIDRs(t *testing.T) {
	cidrs, err := ParseCIDRs([]string{"10.0.0.0/8", "172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}
	ips := []net.IP{net.ParseIP("172.16.1.10")}
	if !anyIPInCIDRs(ips, cidrs) {
		t.Fatalf("IP did not match any CIDR")
	}
}

func TestMultipleIPs(t *testing.T) {
	cidrs, err := ParseCIDRs([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	ips := []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("10.1.2.3")}
	if !anyIPInCIDRs(ips, cidrs) {
		t.Fatalf("no IP matched CIDR")
	}
}

func TestParseCIDRsInvalid(t *testing.T) {
	_, err := ParseCIDRs([]string{"not-a-cidr"})
	if err == nil {
		t.Fatal("expected error")
	}
}
