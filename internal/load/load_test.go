package load

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
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

// h2cAgainst serves handler over unencrypted HTTP/2: every request shares one
// connection, so InFlight requests held at once need one connect, not InFlight.
func h2cAgainst(t *testing.T, handler http.HandlerFunc) (*http.Client, string) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.Config.Protocols = new(http.Protocols)
	server.Config.Protocols.SetUnencryptedHTTP2(true)
	server.Config.HTTP2 = &http.HTTP2Config{MaxConcurrentStreams: 2 * InFlight}
	server.Start()
	t.Cleanup(server.Close)
	transport := &http.Transport{Protocols: new(http.Protocols)}
	transport.Protocols.SetUnencryptedHTTP2(true)
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
	client, url := h2cAgainst(t, func(http.ResponseWriter, *http.Request) {
		arrivals.Add(1)
		if now := time.Now(); now.After(stallFrom) && now.Before(stallUntil) {
			time.Sleep(time.Until(stallUntil))
		}
	})

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
	client, url := h2cAgainst(t, func(http.ResponseWriter, *http.Request) { time.Sleep(time.Second) })

	res, err := Run(context.Background(), config(client, url, 400, 1500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	// 600 due in 1.5s; 256 slots fill by 0.64s and free from 1.0s, each held 1s:
	// about 200 go out late between 1.0s and 1.5s, the rest are unsent.
	if res.Late < 100 || res.Unsent < 50 {
		t.Errorf("late = %d, unsent = %d; want about 200 and 140", res.Late, res.Unsent)
	}
	if res.Sent+res.Unsent != 600 {
		t.Errorf("sent %d + unsent %d != 600 due", res.Sent, res.Unsent)
	}
	// A late request waits at least 0.36s for a slot, then takes 1s.
	if p99 := res.ResponseTimes.Quantile(99); p99 < 1250*time.Millisecond {
		t.Errorf("p99 = %v; latency of a late request must include its wait for a slot", p99)
	}
}

// Nothing is sent after the -load window, however slow the server: the run
// ends once the requests in flight finish.
func TestRun_StopsAtWindow(t *testing.T) {
	var mu sync.Mutex
	var lastArrival time.Time
	client, url := h2cAgainst(t, func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		lastArrival = time.Now()
		mu.Unlock()
		time.Sleep(2 * time.Second)
	})

	start := time.Now()
	res, err := Run(context.Background(), config(client, url, 400, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	mu.Lock()
	sentUntil := lastArrival.Sub(start)
	mu.Unlock()
	if sentUntil > time.Second {
		t.Errorf("a request arrived %v after the start of a 1s load", sentUntil)
	}
	if elapsed > 4*time.Second {
		t.Errorf("Run took %v; want the 1s window plus the 2s in flight", elapsed)
	}
	if res.Sent != InFlight || res.Unsent != 400-InFlight {
		t.Errorf("sent %d, unsent %d; want %d and %d", res.Sent, res.Unsent, InFlight, 400-InFlight)
	}
}

func TestDueCount(t *testing.T) {
	tests := []struct {
		rate float64
		d    time.Duration
		want int
	}{
		{100, 3 * time.Second, 300},
		{400, 1500 * time.Millisecond, 600},
		{3, time.Second, 3},
		{0.5, 3 * time.Second, 2}, // due at 0s and 2s
		{1000, time.Millisecond, 1},
	}
	for _, tt := range tests {
		if got := dueCount(tt.rate, tt.d); got != tt.want {
			t.Errorf("dueCount(%g, %v) = %d, want %d", tt.rate, tt.d, got, tt.want)
		}
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
	if due := int64(dueCount(50, 5*time.Second)); res.Sent+res.Unsent != due {
		t.Errorf("sent %d + unsent %d != %d due", res.Sent, res.Unsent, due)
	}
}

// Requests cut off by an abort are sent but neither successful nor failed;
// the success rate leaves them out, as the validator does.
func TestResults_SuccessRateOfFinishedRequests(t *testing.T) {
	r := &run{hist: stats.NewHistogram(), cancel: func() {}}
	r.sent.Store(10)
	r.successful.Store(6)
	r.failed.Store(2)
	if got := r.results(Config{Rate: 1}, time.Second).SuccessRate; got != 75 {
		t.Errorf("success rate = %v, want 75 (6 of 8 finished)", got)
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

// The last request is due 1ns before the window ends, so its timer wakes after
// the end. A request the loop slept for goes out when a slot is free.
func TestRun_SendsLastRequestOnLateWake(t *testing.T) {
	d := 100*time.Millisecond + time.Nanosecond
	client, url := runAgainst(t, func(http.ResponseWriter, *http.Request) {}, 10, d)
	res, err := Run(context.Background(), config(client, url, 10, d))
	if err != nil {
		t.Fatal(err)
	}
	if res.Sent != 2 || res.Unsent != 0 {
		t.Errorf("sent %d, unsent %d; want 2 and 0", res.Sent, res.Unsent)
	}
}

// A request due inside the window that the loop reaches only after it (a busy
// client) goes out when a slot is free: unsent counts slots held at the end.
func TestRun_SendsDueRequestReachedAfterWindow(t *testing.T) {
	client, url := runAgainst(t, func(http.ResponseWriter, *http.Request) {}, 1, time.Nanosecond)
	res, err := Run(context.Background(), config(client, url, 1, time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if res.Sent != 1 || res.Unsent != 0 {
		t.Errorf("sent %d, unsent %d; want 1 and 0", res.Sent, res.Unsent)
	}
}

// windowRun is what a run against a stalled server observed. Run's start lies
// between before and firstArrival; the first waited-for slot came free after
// released, and the last one before lastFreed.
type windowRun struct {
	res                                       *domain.LoadResults
	before, firstArrival, released, lastFreed time.Time
	waits                                     int
}

// runStalled runs a load against a server that holds every request until
// releaseAt after the first arrival (none when releaseAt is 0). When holdUntil
// is set, the first request that waited for a slot holds the loop until
// holdUntil after the first arrival, as a client stall would.
func runStalled(t *testing.T, rate float64, d, releaseAt, holdUntil time.Duration) windowRun {
	t.Helper()
	var (
		mu          sync.Mutex
		w           windowRun
		arrive      sync.Once
		releaseOnce sync.Once
		release     = make(chan struct{})
	)
	open := func() { releaseOnce.Do(func() { close(release) }) }
	if releaseAt == 0 {
		open()
	}
	client, url := h2cAgainst(t, func(_ http.ResponseWriter, r *http.Request) {
		arrive.Do(func() {
			mu.Lock()
			w.firstArrival = time.Now()
			mu.Unlock()
			if releaseAt > 0 {
				time.AfterFunc(releaseAt, func() {
					mu.Lock()
					w.released = time.Now()
					mu.Unlock()
					open()
				})
			}
		})
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(open) // before server.Close, which waits for the handlers

	cfg := config(client, url, rate, d)
	cfg.afterWait = func() {
		now := time.Now()
		w.waits++
		w.lastFreed = now
		if holdUntil > 0 && w.waits == 1 {
			mu.Lock()
			until := w.firstArrival.Add(holdUntil)
			mu.Unlock()
			time.Sleep(time.Until(until))
		}
	}
	w.before = time.Now()
	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	w.res = res
	return w
}

// A request is late when it waited for a slot or fell due before a waited-for
// slot came free; after the window, those due before it are unsent and the
// rest still go out if a slot is free. The expected counts follow from that,
// with the slot's time bounded by what the run observed.
func TestRun_LateAndUnsentByCause(t *testing.T) {
	exactly := func(n int) func(int, int) (lo, hi int) { return func(int, int) (lo, hi int) { return n, n } }
	// From the first request that waited to the last due before its slot came free.
	backlog := func(dueLo, dueHi int) (lo, hi int) { return dueLo - InFlight, dueHi - InFlight }
	// The same, less the request that waited: it got its slot.
	backlogPastWaiter := func(dueLo, dueHi int) (lo, hi int) { return dueLo - InFlight - 1, dueHi - InFlight - 1 }

	tests := []struct {
		name                 string
		rate                 float64
		d                    time.Duration
		releaseAt, holdUntil time.Duration
		waits                bool
		late, unsent         func(dueLo, dueHi int) (lo, hi int)
	}{
		// 200 requests due in 2µs: the loop reaches most after the window.
		{"client lag only", 1e8, 2 * time.Microsecond, 0, 0, false, exactly(0), exactly(0)},
		// 256 slots fill by 1.024s at 250 req/s.
		{"server stall, recovery inside the window", 250, 1500 * time.Millisecond, 1150 * time.Millisecond, 0, true, backlog, exactly(0)},
		{"server stall across the end", 250, 1500 * time.Millisecond, 2 * time.Second, 0, false, exactly(0), exactly(375 - InFlight)},
		{"server stall, then a client stall across the end", 250, 1500 * time.Millisecond, 1300 * time.Millisecond, 1700 * time.Millisecond, true, exactly(1), backlogPastWaiter},
		{"one wait, then client lag inside the window", 250, 1500 * time.Millisecond, 1150 * time.Millisecond, 1350 * time.Millisecond, true, backlog, exactly(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := runStalled(t, tt.rate, tt.d, tt.releaseAt, tt.holdUntil)
			res := w.res
			if due := int64(dueCount(tt.rate, tt.d)); res.Sent+res.Unsent != due {
				t.Fatalf("sent %d + unsent %d != %d due", res.Sent, res.Unsent, due)
			}
			if tt.waits != (w.waits > 0) {
				t.Fatalf("%d waits for a slot, %d failed", w.waits, res.Failed)
			}
			dueLo, dueHi := 0, 0
			if w.waits > 0 {
				dueLo = dueCount(tt.rate, w.released.Sub(w.firstArrival))
				dueHi = dueCount(tt.rate, w.lastFreed.Sub(w.before))
			}
			seen := fmt.Sprintf("%d waits, %d to %d due before the slot came free, %d failed", w.waits, dueLo, dueHi, res.Failed)
			if lo, hi := tt.late(dueLo, dueHi); res.Late < int64(lo) || res.Late > int64(hi) {
				t.Errorf("late = %d, want %d to %d (%s)", res.Late, lo, hi, seen)
			}
			if lo, hi := tt.unsent(dueLo, dueHi); res.Unsent < int64(lo) || res.Unsent > int64(hi) {
				t.Errorf("unsent = %d, want %d to %d (%s)", res.Unsent, lo, hi, seen)
			}
		})
	}
}

// lateWindow is a window whose deadline, end, has passed but whose timer has
// not fired (no deadline when end is zero); its first Done call frees the
// slot acquire is waiting for.
type lateWindow struct {
	context.Context
	end  time.Time
	free func()
}

func (w lateWindow) Deadline() (time.Time, bool) { return w.end, !w.end.IsZero() }

func (w lateWindow) Done() <-chan struct{} {
	w.free()
	return nil
}

// A slot that comes free after the window is given back, however late the
// window's timer fires: no request waits for a slot past the window.
func TestAcquire_GivesBackSlotAfterWindow(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	var once sync.Once
	window := lateWindow{context.Background(), time.Now(), func() { once.Do(func() { go func() { <-slots }() }) }}
	got, _ := acquire(window, slots, func() { t.Error("afterWait ran for a slot taken after the window") })
	if got || len(slots) != 0 {
		t.Errorf("got %v with %d slots held; want false and 0", got, len(slots))
	}
}

// Without a deadline, a slot that comes free after a wait is kept.
func TestAcquire_KeepsSlotWithoutDeadline(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	var once sync.Once
	window := lateWindow{context.Background(), time.Time{}, func() { once.Do(func() { go func() { <-slots }() }) }}
	waited := 0
	got, freedAt := acquire(window, slots, func() { waited++ })
	if !got || freedAt.IsZero() || waited != 1 || len(slots) != 1 {
		t.Errorf("got %v, freed at %v, afterWait ran %d times, %d slots held; want true, a time, 1 and 1", got, freedAt, waited, len(slots))
	}
}
