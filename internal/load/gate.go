package load

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/util"
)

// AckEnv is the environment variable that must name the target host to confirm ownership.
const AckEnv = "LOBSTER_I_OWN_THIS"

// NeedsAck reports whether a load against a host needs the ownership
// acknowledgement: the host resolves to any address util.IsPrivateAddr does
// not cover, or its robots.txt declares a Crawl-delay. An unresolved host (no
// addresses) needs it too.
func NeedsAck(addrs []netip.Addr, crawlDelay time.Duration) bool {
	if crawlDelay > 0 || len(addrs) == 0 {
		return true
	}
	for _, a := range addrs {
		if !util.IsPrivateAddr(a) {
			return true
		}
	}
	return false
}

// CheckAck returns nil when the acknowledgement is not needed, or when both
// -i-own-this is set and LOBSTER_I_OWN_THIS equals host. -allow-private-ips
// does not count.
func CheckAck(needed, flagSet bool, envValue, host string) error {
	if !needed || (flagSet && envValue == host) {
		return nil
	}
	return fmt.Errorf("-load against %s needs an ownership acknowledgement "+
		"(the host is public or its robots.txt sets a Crawl-delay): "+
		"pass -i-own-this and set %s=%s", host, AckEnv, host)
}
