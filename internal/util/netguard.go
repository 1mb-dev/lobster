package util

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
)

// blockedPrefixes are ranges a crawl must not reach without -allow-private-ips,
// beyond what netip.Addr's Is* methods cover.
var blockedPrefixes = func() []netip.Prefix {
	cidrs := []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // carrier-grade NAT
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved, broadcast
		"64:ff9b::/96",    // NAT64: embeds any IPv4 address, private ones included
		"64:ff9b:1::/48",  // local-use NAT64
		"2002::/16",       // 6to4: embeds any IPv4 address
		"2001::/32",       // Teredo: embeds an IPv4 address
		"2001:db8::/32",   // documentation
		"100::/64",        // discard-only
		"::/96",           // IPv4-compatible (deprecated): embeds an IPv4 address
	}
	prefixes := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		prefixes[i] = netip.MustParsePrefix(c)
	}
	return prefixes
}()

// IsBlockedAddr reports whether addr is loopback, private, link-local,
// multicast, unspecified or in a special-purpose range. IPv4-mapped IPv6
// addresses are judged as their IPv4 address.
func IsBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IsLocalhostName reports whether hostname is "localhost" or a name under .localhost.
func IsLocalhostName(hostname string) bool {
	h := strings.TrimSuffix(strings.ToLower(hostname), ".")
	return h == "localhost" || h == "localhost.localdomain" || strings.HasSuffix(h, ".localhost")
}

// BlockedAddressError is returned when a dial targets a blocked address.
type BlockedAddressError struct {
	Addr netip.Addr
}

func (e *BlockedAddressError) Error() string {
	return fmt.Sprintf("connection to private or reserved address %s refused (use -allow-private-ips for a service you own)", e.Addr)
}

// DialControl returns a net.Dialer Control function that refuses connections
// to blocked addresses. It runs after DNS resolution, on the address actually
// dialed, so a hostname that resolves (or rebinds) to a private IP is caught.
// With allowPrivate it refuses nothing.
func DialControl(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("dial address %q: %w", address, err)
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("dial address %q: %w", address, err)
		}
		if IsBlockedAddr(addr) {
			return &BlockedAddressError{Addr: addr.Unmap()}
		}
		return nil
	}
}

// PublicAddressError is returned when a private-only dial targets a public address.
type PublicAddressError struct {
	Addr netip.Addr
}

func (e *PublicAddressError) Error() string {
	return fmt.Sprintf("connection to public address %s refused: the load was not acknowledged for a public host", e.Addr)
}

// PrivateOnlyDialControl returns a net.Dialer Control function that refuses
// every address IsBlockedAddr does not cover. A load that needed no ownership
// acknowledgement uses it, so a host that resolves to a public address later
// (DNS rebinding) is refused at dial time.
func PrivateOnlyDialControl() func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("dial address %q: %w", address, err)
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("dial address %q: %w", address, err)
		}
		if !IsBlockedAddr(addr) {
			return &PublicAddressError{Addr: addr.Unmap()}
		}
		return nil
	}
}
