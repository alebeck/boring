package vpn

import (
	"fmt"
	"net"
)

// ParseCIDRs parses CIDR strings used to detect whether the machine is on VPN.
func ParseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	parsed := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid VPN CIDR '%v': %w", cidr, err)
		}
		parsed = append(parsed, ipNet)
	}
	return parsed, nil
}

func interfaceIPs() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for _, iface := range interfaces {
		isUp := iface.Flags&net.FlagUp != 0
		isLoopback := iface.Flags&net.FlagLoopback != 0
		if !isUp || isLoopback {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("could not read addresses for interface '%v': %w", iface.Name, err)
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil {
				continue
			}
			ips = append(ips, normalizeIP(ip))
		}
	}
	return ips, nil
}

// OnVPN reports whether any active local interface address is inside any CIDR.
func OnVPN(cidrs []*net.IPNet) (bool, error) {
	ips, err := interfaceIPs()
	if err != nil {
		return false, err
	}
	return anyIPInCIDRs(ips, cidrs), nil
}

func anyIPInCIDRs(ips []net.IP, cidrs []*net.IPNet) bool {
	for _, ip := range ips {
		for _, cidr := range cidrs {
			if cidr.Contains(normalizeIP(ip)) {
				return true
			}
		}
	}
	return false
}

func ipFromAddr(addr net.Addr) net.IP {
	switch a := addr.(type) {
	case *net.IPNet:
		return a.IP
	case *net.IPAddr:
		return a.IP
	default:
		return nil
	}
}

func normalizeIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}
