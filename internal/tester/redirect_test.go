package tester

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/util"
)

// otherHost serves on 127.0.0.1 and returns its URL, target, as http://localhost:<port>,
// a different hostname from the 127.0.0.1 test servers. hits counts requests;
// sawHeader is set if a request carried X-Api-Key.
func otherHost(t *testing.T) (target string, hits *atomic.Int64, sawHeader *atomic.Bool) {
	t.Helper()
	hits, sawHeader = new(atomic.Int64), new(atomic.Bool)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("X-Api-Key") != "" {
			sawHeader.Store(true)
		}
	}))
	t.Cleanup(server.Close)
	return strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/", hits, sawHeader
}

func runTester(t *testing.T, config domain.TesterConfig) (*domain.TestResults, error) {
	t.Helper()
	config.NoProgress = true
	tr, err := New(config, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return tr.Run(ctx)
}

func TestRedirect_BaseToOtherHostIsFatal(t *testing.T) {
	target, hits, _ := otherHost(t)
	base := httptest.NewServer(http.RedirectHandler(target, http.StatusFound))
	defer base.Close()

	_, err := runTester(t, testConfig(base.URL+"/"))
	var refused *RedirectRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("Run error = %v, want a RedirectRefusedError", err)
	}
	if !strings.Contains(err.Error(), target) || !strings.Contains(err.Error(), "-url") {
		t.Errorf("error %q should name the final URL %s and point to -url", err, target)
	}
	if hits.Load() != 0 {
		t.Errorf("the other host got %d requests, want 0", hits.Load())
	}
}

func TestRedirect_OffHostPageFailsWithoutLeakingHeaders(t *testing.T) {
	target, hits, sawHeader := otherHost(t)
	s := &site{pages: map[string][]string{"/": {"/go"}}}
	s.hits = map[string]int{}
	mux := http.NewServeMux()
	mux.Handle("/go", http.RedirectHandler(target, http.StatusFound))
	mux.Handle("/", s)
	base := httptest.NewServer(mux)
	defer base.Close()

	config := testConfig(base.URL + "/")
	config.Auth = &domain.AuthConfig{Type: "header", Headers: map[string]string{"X-Api-Key": "secret"}}
	r, err := runTester(t, config)
	if err != nil {
		t.Fatalf("Run: %v (a refused redirect below the base page is a failed request, not fatal)", err)
	}
	if r.TotalRequests != 2 || r.FailedRequests != 1 {
		t.Errorf("total/failed = %d/%d, want 2/1", r.TotalRequests, r.FailedRequests)
	}
	if hits.Load() != 0 || sawHeader.Load() {
		t.Errorf("other host: %d requests, header seen %v; want none", hits.Load(), sawHeader.Load())
	}
}

func TestRedirect_RobotsRecheckedAndSameHostFollowed(t *testing.T) {
	s := &site{
		pages:  map[string][]string{"/": {"/go-private", "/old"}, "/new": nil, "/private": nil},
		robots: "User-agent: *\nDisallow: /private\n",
	}
	s.hits = map[string]int{}
	mux := http.NewServeMux()
	mux.Handle("/go-private", http.RedirectHandler("/private", http.StatusFound))
	mux.Handle("/old", http.RedirectHandler("/new", http.StatusMovedPermanently))
	mux.Handle("/", s)
	base := httptest.NewServer(mux)
	defer base.Close()

	config := testConfig(base.URL + "/")
	config.IgnoreRobots = false
	r, err := runTester(t, config)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, hits := s.pageHits(); hits["/private"] != 0 || hits["/new"] != 1 {
		t.Errorf("/private fetched %d times (want 0), /new %d (want 1)", hits["/private"], hits["/new"])
	}
	if r.FailedRequests != 1 {
		t.Errorf("failed = %d, want 1 (the redirect into a disallowed path)", r.FailedRequests)
	}
}

func TestRedirect_Capped(t *testing.T) {
	var hits atomic.Int64
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/r%d", n), http.StatusFound)
	}))
	defer base.Close()

	_, err := runTester(t, testConfig(base.URL+"/"))
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d redirects", maxRedirects)) {
		t.Fatalf("Run error = %v, want the redirect cap", err)
	}
	if hits.Load() != maxRedirects {
		t.Errorf("server saw %d requests, want %d", hits.Load(), maxRedirects)
	}
}

func TestDial_PrivateBaseRefusedWithoutAllow(t *testing.T) {
	base := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer base.Close()

	config := testConfig(base.URL + "/")
	config.AllowPrivateIPs = false
	_, err := runTester(t, config)
	var blocked *util.BlockedAddressError
	if !errors.As(err, &blocked) {
		t.Fatalf("Run error = %v, want a BlockedAddressError for 127.0.0.1", err)
	}
}

func TestTransport_NoProxyCheckedDialer(t *testing.T) {
	tr, err := New(testConfig("http://127.0.0.1:1/"), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := tr.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.DialContext == nil {
		t.Errorf("crawl transport: %#v; want no proxy and a checked dialer", tr.client.Transport)
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
		if got := sameOrigin(base, target); got != tt.want {
			t.Errorf("sameOrigin(%s, %s) = %v, want %v", tt.base, tt.target, got, tt.want)
		}
	}
}

func TestRedirect_OtherPortSameHostRefused(t *testing.T) {
	var otherHits atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { otherHits.Add(1) }))
	defer other.Close()
	base := httptest.NewServer(http.RedirectHandler(other.URL+"/", http.StatusFound)) // same 127.0.0.1, another port
	defer base.Close()

	_, err := runTester(t, testConfig(base.URL+"/"))
	var refused *RedirectRefusedError
	if !errors.As(err, &refused) || otherHits.Load() != 0 {
		t.Fatalf("Run error = %v, other port hit %d times; want a refusal and no request", err, otherHits.Load())
	}
}

func TestRedirect_RefusalHidesQuerySecrets(t *testing.T) {
	target, _, _ := otherHost(t)
	base := httptest.NewServer(http.RedirectHandler(target+"?token=s3cr3t-tok", http.StatusFound))
	defer base.Close()

	_, err := runTester(t, testConfig(base.URL+"/"))
	if err == nil || strings.Contains(err.Error(), "s3cr3t-tok") {
		t.Fatalf("Run error = %v; want a refusal without the query token", err)
	}
}

// A load that needed no acknowledgement must not reach a public address, even
// if the host's DNS changes after the gate's check. The refusal happens in
// Control, before any packet is sent.
func TestLoadClient_PrivateOnlyRefusesPublic(t *testing.T) {
	tr, err := New(testConfig("http://127.0.0.1:1/"), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.LoadClient(4, true).Get("http://93.184.216.34:9/")
	var public *util.PublicAddressError
	if !errors.As(err, &public) {
		t.Fatalf("Get public address = %v, want a PublicAddressError", err)
	}
}

// privateOnly adds to the crawl's check: without -allow-private-ips a private
// address stays refused, so no address is reachable.
func TestLoadClient_PrivateOnlyKeepsCrawlCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	config := testConfig("http://127.0.0.1:1/")
	config.AllowPrivateIPs = false
	tr, err := New(config, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.LoadClient(4, true).Get(server.URL)
	var blocked *util.BlockedAddressError
	if !errors.As(err, &blocked) {
		t.Fatalf("Get 127.0.0.1 without -allow-private-ips = %v, want a BlockedAddressError", err)
	}
}
