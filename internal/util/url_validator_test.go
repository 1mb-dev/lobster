//nolint:revive // util is an established package name pattern
package util

import (
	"net/url"
	"strings"
	"testing"
)

func TestValidateBaseURL_ValidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"simple http", "http://example.com"},
		{"simple https", "https://example.com"},
		{"with port", "https://example.com:8080"},
		{"with path", "https://example.com/api/v1"},
		{"with query", "https://example.com?foo=bar"},
		{"with fragment", "https://example.com#section"},
		{"subdomain", "https://api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, false)
			if err != nil {
				t.Errorf("ValidateBaseURL(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}

func TestValidateBaseURL_InvalidSchemes(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		reason string
	}{
		{"file scheme", "file:///etc/passwd", "unsupported scheme"},
		{"ftp scheme", "ftp://ftp.example.com", "unsupported scheme"},
		{"gopher scheme", "gopher://example.com", "unsupported scheme"},
		{"javascript scheme", "javascript:alert(1)", "unsupported scheme"},
		{"data scheme", "data:text/html,<script>alert(1)</script>", "unsupported scheme"},
		{"no scheme", "example.com", "unsupported scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, false)
			if err == nil {
				t.Errorf("ValidateBaseURL(%q) = nil, want error", tt.url)
				return
			}
			if !strings.Contains(err.Error(), tt.reason) {
				t.Errorf("ValidateBaseURL(%q) error = %v, want error containing %q", tt.url, err, tt.reason)
			}
		})
	}
}

func TestValidateBaseURL_PrivateIPs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost"},
		{"localhost with port", "http://localhost:8080"},
		{"127.0.0.1", "http://127.0.0.1"},
		{"127.0.0.1 with port", "http://127.0.0.1:3000"},
		{"10.x.x.x", "http://10.0.0.1"},
		{"172.16.x.x", "http://172.16.0.1"},
		{"172.31.x.x", "http://172.31.255.255"},
		{"192.168.x.x", "http://192.168.1.1"},
		{"0.0.0.0", "http://0.0.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name+" blocked", func(t *testing.T) {
			err := ValidateBaseURL(tt.url, false)
			if err == nil {
				t.Errorf("ValidateBaseURL(%q, allowPrivateIPs=false) = nil, want error", tt.url)
			}
		})

		t.Run(tt.name+" allowed", func(t *testing.T) {
			err := ValidateBaseURL(tt.url, true)
			if err != nil {
				t.Errorf("ValidateBaseURL(%q, allowPrivateIPs=true) = %v, want nil", tt.url, err)
			}
		})
	}
}

func TestValidateBaseURL_EmptyAndMalformed(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		reason string
	}{
		{"empty string", "", "cannot be empty"},
		{"just scheme", "http://", "missing host"},
		{"missing host", "http://:8080", "missing hostname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, false)
			if err == nil {
				t.Errorf("ValidateBaseURL(%q) = nil, want error", tt.url)
				return
			}
			if !strings.Contains(err.Error(), tt.reason) {
				t.Errorf("ValidateBaseURL(%q) error = %v, want error containing %q", tt.url, err, tt.reason)
			}
		})
	}
}

func TestValidateBaseURL_SpecialIPRanges(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"carrier-grade NAT 100.64.0.0", "http://100.64.0.1", true},
		{"carrier-grade NAT 100.127.255.255", "http://100.127.255.255", true},
		{"outside carrier-grade NAT", "http://100.128.0.1", false},
		{"TEST-NET-1", "http://192.0.2.1", true},
		{"TEST-NET-2", "http://198.51.100.1", true},
		{"TEST-NET-3", "http://203.0.113.1", true},
		{"multicast 224.x", "http://224.0.0.1", true},
		{"multicast 239.x", "http://239.255.255.255", true},
		{"reserved 240.x", "http://240.0.0.1", true},
		{"reserved 255.x", "http://255.255.255.255", true},
		{"benchmarking 198.18.x", "http://198.18.0.1", true},
		{"IPv4-mapped loopback", "http://[::ffff:127.0.0.1]", true},
		{"NAT64 loopback", "http://[64:ff9b::7f00:1]", true},
		{"6to4 loopback", "http://[2002:7f00:1::]", true},
		{".localhost name", "http://foo.localhost", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, false)
			if tt.blocked && err == nil {
				t.Errorf("ValidateBaseURL(%q) = nil, want blocked", tt.url)
			}
			if !tt.blocked && err != nil {
				t.Errorf("ValidateBaseURL(%q) = %v, want allowed", tt.url, err)
			}
		})
	}
}

func TestURLValidationError_Error(t *testing.T) {
	err := &URLValidationError{
		URL:    "file:///etc/passwd",
		Reason: "unsupported scheme",
	}
	expected := `invalid URL "file:///etc/passwd": unsupported scheme`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		base, target string
		want         bool
	}{
		{"http://example.com/", "http://example.com/a", true},
		{"http://example.com/", "http://EXAMPLE.com:80/a", true},
		{"http://example.com/", "https://example.com/a", true}, // upgrade 80→443
		{"https://example.com/", "https://example.com:443/a", true},
		{"https://example.com/", "http://example.com/a", false},       // downgrade
		{"https://example.com/", "https://example.com:8443/a", false}, // another port
		{"http://example.com:8080/", "http://example.com/a", false},
		{"http://example.com:8080/", "https://example.com/a", false}, // not the 80→443 upgrade
		{"http://example.com/", "http://www.example.com/a", false},
	}
	for _, tt := range tests {
		base, _ := url.Parse(tt.base)
		target, _ := url.Parse(tt.target)
		if got := SameOrigin(base, target); got != tt.want {
			t.Errorf("SameOrigin(%s, %s) = %v, want %v", tt.base, tt.target, got, tt.want)
		}
	}
}
