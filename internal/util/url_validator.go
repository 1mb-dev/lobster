package util

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// URLValidationError represents a URL validation failure
type URLValidationError struct {
	URL    string
	Reason string
}

func (e *URLValidationError) Error() string {
	return fmt.Sprintf("invalid URL %q: %s", e.URL, e.Reason)
}

// ValidateBaseURL validates a URL for use as a load test target.
// It checks for:
// - Valid URL syntax
// - HTTP or HTTPS scheme only (blocks file://, ftp://, gopher://, etc.)
// - Non-empty host
// - Optional: blocks localhost names and literal private IPs unless allowPrivateIPs is true
func ValidateBaseURL(rawURL string, allowPrivateIPs bool) error {
	if rawURL == "" {
		return &URLValidationError{URL: rawURL, Reason: "URL cannot be empty"}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &URLValidationError{URL: rawURL, Reason: fmt.Sprintf("invalid URL syntax: %v", err)}
	}

	// Validate scheme
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return &URLValidationError{
			URL:    rawURL,
			Reason: fmt.Sprintf("unsupported scheme %q (only http and https allowed)", parsed.Scheme),
		}
	}

	// Validate host is present
	if parsed.Host == "" {
		return &URLValidationError{URL: rawURL, Reason: "missing host"}
	}

	// Extract hostname (without port)
	hostname := parsed.Hostname()
	if hostname == "" {
		return &URLValidationError{URL: rawURL, Reason: "missing hostname"}
	}

	// Check for private/localhost IPs unless allowed
	if !allowPrivateIPs {
		if err := validateNotPrivateIP(hostname); err != nil {
			return &URLValidationError{URL: rawURL, Reason: err.Error()}
		}
	}

	return nil
}

// validateNotPrivateIP refuses localhost names and literal blocked IPs. A
// hostname is not resolved here: DialControl checks every address at dial time.
func validateNotPrivateIP(hostname string) error {
	if IsLocalhostName(hostname) {
		return fmt.Errorf("localhost is blocked (use -allow-private-ips for a service you own)")
	}
	if addr, err := netip.ParseAddr(hostname); err == nil && IsBlockedAddr(addr) {
		return fmt.Errorf("private or reserved IP %s is blocked (use -allow-private-ips for a service you own)", addr)
	}
	return nil
}
