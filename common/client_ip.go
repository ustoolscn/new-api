package common

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

func ResolveClientIP(remoteAddr, forwardedFor, realIP string, trustedProxies []netip.Prefix) (netip.Addr, error) {
	peer, err := parseRemoteAddr(remoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	if !isAddrInPrefixes(peer, trustedProxies) {
		return peer, nil
	}

	forwardedFor = strings.TrimSpace(forwardedFor)
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		chain := make([]netip.Addr, 0, len(parts))
		for _, part := range parts {
			value := strings.TrimSpace(part)
			addr, parseErr := netip.ParseAddr(value)
			if parseErr != nil {
				return netip.Addr{}, fmt.Errorf("invalid X-Forwarded-For address %q", value)
			}
			chain = append(chain, addr.Unmap())
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !isAddrInPrefixes(chain[i], trustedProxies) {
				return chain[i], nil
			}
		}
		return chain[0], nil
	}

	realIP = strings.TrimSpace(realIP)
	if realIP == "" {
		return peer, nil
	}
	addr, err := netip.ParseAddr(realIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid X-Real-IP address %q", realIP)
	}
	return addr.Unmap(), nil
}

func parseRemoteAddr(remoteAddr string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		addr, parseErr := netip.ParseAddr(host)
		if parseErr != nil {
			return netip.Addr{}, fmt.Errorf("invalid remote address %q", remoteAddr)
		}
		return addr.Unmap(), nil
	}

	addr, parseErr := netip.ParseAddr(strings.TrimSpace(remoteAddr))
	if parseErr != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q", remoteAddr)
	}
	return addr.Unmap(), nil
}

func isAddrInPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
