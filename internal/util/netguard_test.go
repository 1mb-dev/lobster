package util

import (
	"errors"
	"net/netip"
	"testing"
)

func TestIsBlockedAddr(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.255.255.254", "::1",
		"10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", "fc00::1", "fd12:3456::1",
		"169.254.169.254", "fe80::1",
		"0.0.0.0", "0.1.2.3", "::",
		"100.64.0.1", "100.127.255.255",
		"192.0.0.1", "192.0.2.1", "198.51.100.1", "203.0.113.1",
		"198.18.0.1", "198.19.255.255", // benchmarking 198.18/15
		"224.0.0.1", "239.255.255.255", "ff02::1",
		"240.0.0.1", "255.255.255.255",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", // IPv4-mapped (4-in-6)
		"::127.0.0.1",                       // IPv4-compatible
		"64:ff9b::7f00:1", "64:ff9b::a00:1", // NAT64 of 127.0.0.1 and 10.0.0.1
		"64:ff9b:1::1",                   // local-use NAT64
		"2002:7f00:1::", "2002:a00:1::1", // 6to4 of 127.0.0.1 and 10.0.0.1
		"2001:0:4136:e378::1", // Teredo
		"2001:db8::1",         // documentation
		"100::1",              // discard-only
	}
	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34", "100.128.0.1", "198.20.0.1", "172.32.0.1",
		"2606:4700:4700::1111", "2001:4860:4860::8888", "::ffff:8.8.8.8",
	}
	for _, s := range blocked {
		if !IsBlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("IsBlockedAddr(%s) = false, want true", s)
		}
	}
	for _, s := range allowed {
		if IsBlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("IsBlockedAddr(%s) = true, want false", s)
		}
	}
}

func TestIsLocalhostName(t *testing.T) {
	for _, h := range []string{"localhost", "LOCALHOST", "localhost.", "foo.localhost", "a.b.localhost.", "localhost.localdomain"} {
		if !IsLocalhostName(h) {
			t.Errorf("IsLocalhostName(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"localhost.example.com", "notlocalhost", "example.com", "mylocalhost"} {
		if IsLocalhostName(h) {
			t.Errorf("IsLocalhostName(%q) = true, want false", h)
		}
	}
}

func TestDialControl(t *testing.T) {
	refuse := DialControl(false)
	var blockedErr *BlockedAddressError
	for _, addr := range []string{"127.0.0.1:80", "[::1]:443", "[::ffff:10.0.0.1]:80", "[64:ff9b::7f00:1]:80"} {
		if err := refuse("tcp", addr, nil); !errors.As(err, &blockedErr) {
			t.Errorf("DialControl(false)(%s) = %v, want a BlockedAddressError", addr, err)
		}
	}
	if err := refuse("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("DialControl(false) refused a public address: %v", err)
	}
	if err := DialControl(true)("tcp", "127.0.0.1:80", nil); err != nil {
		t.Errorf("DialControl(true) refused loopback: %v", err)
	}
}

func TestPrivateOnlyDialControl(t *testing.T) {
	control := PrivateOnlyDialControl()
	var public *PublicAddressError
	for _, addr := range []string{"93.184.216.34:80", "[2606:4700:4700::1111]:443", "[::ffff:8.8.8.8]:80"} {
		if err := control("tcp", addr, nil); !errors.As(err, &public) {
			t.Errorf("PrivateOnlyDialControl(%s) = %v, want a PublicAddressError", addr, err)
		}
	}
	for _, addr := range []string{"127.0.0.1:80", "[::1]:80", "10.0.0.1:443"} {
		if err := control("tcp", addr, nil); err != nil {
			t.Errorf("PrivateOnlyDialControl(%s) refused a private address: %v", addr, err)
		}
	}
}
