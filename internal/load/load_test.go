package load

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/stats"
)

type plainRequester struct{}

func (plainRequester) NewRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
}

func runAgainst(t *testing.T, handler http.HandlerFunc, rate float64, d time.Duration) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{MaxConnsPerHost: InFlight, MaxIdleConnsPerHost: InFlight}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, server.URL + "/"
}

func config(client *http.Client, url string, rate float64, d time.Duration) Config {
	return Config{Client: client, Requester: plainRequester{}, URLs: []string{url, url + "a"}, Rate: rate, Duration: d}
}

func within(got, want, pct float64) bool {
	return got >= want*(1-pct/100) && got <= want*(1+pct/100)
}

func TestRun_RateWithin5Percent(t *testing.T) {
	var arrivals atomic.Int64
	client, url := runAgainst(t, func(http.ResponseWriter, *http.Request) { arrivals.Add(1) }, 100, 3*time.Second)

	res, err := Run(context.Background(), config(client, url, 100, 3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !within(float64(arrivals.Load()), 300, 5) || !within(res.AchievedRate, 100, 5) {
		t.Errorf("server saw %d requests, achieved %.1f req/s; want 300 and 100 within 5%%", arrivals.Load(), res.AchievedRate)
	}
	if res.Successful != res.Sent || res.Late != 0 || res.Aborted {
		t.Errorf("results = %+v", res)
	}
}

// A 2s stall must show in p99 and must not lower the arrival rate: the
// schedule does not wait for responses.
func TestRun_StallShowsInP99NotInRate(t *testing.T) {
	start := time.Now()
	stallFrom, stallUntil := start.Add(500*time.Millisecond), start.Add(2500*time.Millisecond)
	var arrivals atomic.Int64
	client, url := runAgainst(t, func(http.ResponseWriter, *http.Request) {
		arrivals.Add(1)
		if now := time.Now(); now.After(stallFrom) && now.Before(stallUntil) {
			time.Sleep(time.Until(stallUntil))
		}
	}, 100, 4*time.Second)

	res, err := Run(context.Background(), config(client, url, 100, 4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !within(float64(arrivals.Load()), 400, 5) {
		t.Errorf("server saw %d requests in 4s at 100 req/s; the stall lowered the rate", arrivals.Load())
	}
	if p99 := res.ResponseTimes.Quantile(99); p99 < 1500*time.Millisecond {
		t.Errorf("p99 = %v, want the 2s stall to show (>= 1.5s)", p99)
	}
	if res.Late != 0 {
		t.Errorf("late = %d; 200 in flight is under the %d cap", res.Late, InFlight)
	}
}

func TestRun_LateWhenSlotsFull(t *testing.T) {
	client, url := runAgainst(t, func(http.ResponseWriter, *http.Request) { time.Sleep(time.Second) }, 400, time.Second)

	res, err := Run(context.Background(), config(client, url, 400, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	// 400 requests, 256 slots, each held 1s: about 144 must wait for a slot.
	if res.Late < 100 {
		t.Errorf("late = %d, want about 144 once %d slots are full", res.Late, InFlight)
	}
	// A late request waits about 0.36s for a slot, then takes 1s: its latency
	// from the intended time is about 1.36s, where service time alone is 1s.
	if p99 := res.ResponseTimes.Quantile(99); p99 < 1250*time.Millisecond {
		t.Errorf("p99 = %v; latency of a late request must include its wait for a slot", p99)
	}
}

func TestRun_AbortsOnConsecutive5xx(t *testing.T) {
	client, url := runAgainst(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }, 50, 5*time.Second)

	start := time.Now()
	res, err := Run(context.Background(), config(client, url, 50, 5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Aborted || res.AbortReason == "" || time.Since(start) > 2*time.Second {
		t.Errorf("aborted=%v reason=%q after %v; want an early abort", res.Aborted, res.AbortReason, time.Since(start))
	}
	if res.Failed < AbortAfter || res.Successful != 0 {
		t.Errorf("failed=%d successful=%d", res.Failed, res.Successful)
	}
}

func TestRecord_DoesNotAllocate(t *testing.T) {
	r := &run{hist: stats.NewHistogram(), cancel: func() {}}
	allocs := testing.AllocsPerRun(1000, func() {
		r.record(42*time.Millisecond, http.StatusOK)
		r.record(42*time.Millisecond, http.StatusNotFound)
	})
	if allocs != 0 {
		t.Errorf("record allocates %.1f times per call; results must not grow with requests", allocs)
	}
}

func TestNeedsAck(t *testing.T) {
	public := netip.MustParseAddr("93.184.216.34")
	loopback := netip.MustParseAddr("127.0.0.1")
	tests := []struct {
		name  string
		addrs []netip.Addr
		delay time.Duration
		want  bool
	}{
		{"public address", []netip.Addr{public}, 0, true},
		{"private and public", []netip.Addr{loopback, public}, 0, true},
		{"private only", []netip.Addr{loopback}, 0, false},
		{"private with Crawl-delay", []netip.Addr{loopback}, time.Second, true},
		{"unresolved", nil, 0, true},
	}
	for _, tt := range tests {
		if got := NeedsAck(tt.addrs, tt.delay); got != tt.want {
			t.Errorf("%s: NeedsAck = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCheckAck(t *testing.T) {
	if CheckAck(false, false, "", "example.com") != nil {
		t.Error("refused a load that needs no acknowledgement")
	}
	if CheckAck(true, true, "example.com", "example.com") != nil {
		t.Error("refused flag plus matching env")
	}
	for _, c := range []struct {
		flag bool
		env  string
	}{{false, "example.com"}, {true, ""}, {true, "other.com"}} {
		if CheckAck(true, c.flag, c.env, "example.com") == nil {
			t.Errorf("flag=%v env=%q accepted", c.flag, c.env)
		}
	}
}
