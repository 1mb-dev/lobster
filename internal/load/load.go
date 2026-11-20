// Package load replays a crawled URL set open-loop at a fixed rate.
package load

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/stats"
)

// Limits on a load run (the rate ceiling is domain.MaxLoadRate).
const (
	// InFlight is the most requests in flight at once.
	InFlight = 256
	// AbortAfter is how many consecutive 429, 5xx, connection errors or refusals stop the load.
	AbortAfter = 10
)

// Requester builds a request with the run's user agent and authentication.
type Requester interface {
	NewRequest(ctx context.Context, rawURL string) (*http.Request, error)
}

// Config describes one load run.
type Config struct {
	Client    *http.Client
	Requester Requester
	URLs      []string
	Rate      float64
	Duration  time.Duration

	// afterWait, when set, runs after a request has waited for a slot and
	// before it is sent. Tests use it to hold the loop past the window.
	afterWait func()
}

// run holds a load run's counters; record is safe for concurrent use.
type run struct {
	mu         sync.Mutex
	hist       *stats.Histogram
	sent       atomic.Int64
	successful atomic.Int64
	failed     atomic.Int64
	late       atomic.Int64
	consec     atomic.Int64
	unsent     int64

	abortOnce   sync.Once
	abortReason string
	cancel      context.CancelFunc
}

// record adds one finished request: its latency from the intended send time
// and its status (0 when the request or reading its body failed). It does not
// allocate.
func (r *run) record(latency time.Duration, status int) {
	r.mu.Lock()
	r.hist.Record(latency)
	r.mu.Unlock()

	if status >= 200 && status < 400 {
		r.successful.Add(1)
		r.consec.Store(0)
		return
	}
	r.failed.Add(1)
	if status == 0 || status == http.StatusTooManyRequests || status >= 500 {
		if r.consec.Add(1) >= AbortAfter {
			r.abort(fmt.Sprintf("%d consecutive 429, 5xx, connection errors or refusals", AbortAfter))
		}
		return
	}
	r.consec.Store(0)
}

func (r *run) abort(reason string) {
	r.abortOnce.Do(func() {
		r.abortReason = reason
		r.cancel()
	})
}

// Run sends requests to cfg.URLs in turn, one every 1/cfg.Rate seconds, for
// cfg.Duration. The schedule does not wait for responses (open loop): each
// request's latency runs from its intended send time, so a slow server shows
// in the latency instead of lowering the request rate. When all InFlight slots
// are busy, a request waits for one; it and every request after it due before
// that slot came free are counted late. No request due after cfg.Duration is sent, and
// none waits for a slot after it: a request not sent by then goes out only if
// it fell due after the latest waited-for slot came free and a slot is free,
// and is otherwise counted unsent. The run ends once the requests in flight
// finish (within the client's timeout).
func Run(ctx context.Context, cfg Config) (*domain.LoadResults, error) {
	if len(cfg.URLs) == 0 {
		return nil, fmt.Errorf("load: no URLs to replay")
	}
	if cfg.Rate <= 0 {
		return nil, fmt.Errorf("load: rate must be > 0, got %g", cfg.Rate)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := &run{hist: stats.NewHistogram(), cancel: cancel}
	slots := make(chan struct{}, InFlight)
	var wg sync.WaitGroup

	start := time.Now()
	end := start.Add(cfg.Duration)
	// The wait for a slot ends at end.
	windowCtx, windowCancel := context.WithDeadline(ctx, end)
	defer windowCancel()
	timer := time.NewTimer(0) // Reset without a drain needs Go 1.23+ timers (go.mod)
	defer timer.Stop()

	// backlogUntil is when the loop got its latest waited-for slot. Requests
	// due before it are late, and none of them goes out after end; a later one
	// reached after end through this loop's own lag still goes out.
	var backlogUntil time.Time
	due := 0
	for ; ; due++ {
		// Request i is due at start + i/rate whatever happened before it: a
		// token bucket would drop the sends missed while waiting for a slot,
		// hiding the backlog (coordinated omission).
		intended := start.Add(time.Duration(float64(due) * float64(time.Second) / cfg.Rate))
		if !intended.Before(end) {
			break
		}
		if !sleepUntil(ctx, timer, intended) {
			break
		}
		if intended.Before(backlogUntil) && !time.Now().Before(end) {
			continue // a backlog not drained inside the window is unsent
		}
		got, freedAt := acquire(windowCtx, slots, cfg.afterWait)
		if !got {
			break
		}
		if freedAt.After(backlogUntil) {
			backlogUntil = freedAt
		}
		if !freedAt.IsZero() || intended.Before(backlogUntil) {
			r.late.Add(1)
		}

		r.sent.Add(1)
		wg.Add(1)
		go func(rawURL string, intended time.Time) {
			defer wg.Done()
			defer func() { <-slots }()
			status := send(ctx, cfg, rawURL)
			if status == 0 && ctx.Err() != nil {
				return // cut off by an abort: sent, but neither successful nor failed
			}
			r.record(time.Since(intended), status)
		}(cfg.URLs[due%len(cfg.URLs)], intended)
	}
	stopped := time.Since(start)
	wg.Wait() // abortReason is written by request goroutines; read it after they finish

	window := cfg.Duration
	if r.abortReason != "" {
		window = stopped
	}
	r.unsent = int64(dueCount(cfg.Rate, cfg.Duration)) - r.sent.Load()
	return r.results(cfg, window), nil
}

// acquire takes an in-flight slot, waiting for one until windowCtx's deadline,
// and runs afterWait (if set) when it got one after waiting. It reports
// whether it got one and, if it had to wait, when the slot came free (zero
// otherwise).
func acquire(windowCtx context.Context, slots chan struct{}, afterWait func()) (got bool, freedAt time.Time) {
	select {
	case slots <- struct{}{}:
		return true, time.Time{}
	default:
	}
	select {
	case slots <- struct{}{}:
	case <-windowCtx.Done():
		return false, time.Time{}
	}
	freedAt = time.Now()
	// The deadline's timer can fire late, and select picks at random when
	// both cases are ready: a slot taken after the window is given back.
	if end, ok := windowCtx.Deadline(); windowCtx.Err() != nil || ok && !freedAt.Before(end) {
		<-slots
		return false, time.Time{}
	}
	if afterWait != nil {
		afterWait()
	}
	return true, freedAt
}

// dueCount is how many requests the schedule puts before d: those with i/rate < d.
func dueCount(rate float64, d time.Duration) int {
	n := int(rate * d.Seconds())
	for time.Duration(float64(n)*float64(time.Second)/rate) < d {
		n++
	}
	for n > 0 && time.Duration(float64(n-1)*float64(time.Second)/rate) >= d {
		n--
	}
	return n
}

// sleepUntil waits until t; it reports false if ctx ends first.
func sleepUntil(ctx context.Context, timer *time.Timer, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer.Reset(d)
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// send performs one request, reads up to 1MB of its body, and returns its
// status, or 0 when the request fails or reading the body fails.
func send(ctx context.Context, cfg Config, rawURL string) int {
	req, err := cfg.Requester.NewRequest(ctx, rawURL)
	if err != nil {
		return 0
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return 0
	}
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil {
		return 0
	}
	return resp.StatusCode
}

func (r *run) results(cfg Config, scheduled time.Duration) *domain.LoadResults {
	s := r.hist.Summary()
	res := &domain.LoadResults{
		ResponseTimes:       r.hist,
		Duration:            scheduled.Round(time.Millisecond).String(),
		AbortReason:         r.abortReason,
		AverageResponseTime: s.Mean.String(),
		P95ResponseTime:     s.P95.String(),
		P99ResponseTime:     s.P99.String(),
		TargetRate:          cfg.Rate,
		Sent:                r.sent.Load(),
		Successful:          r.successful.Load(),
		Failed:              r.failed.Load(),
		Late:                r.late.Load(),
		Unsent:              r.unsent,
		Aborted:             r.abortReason != "",
	}
	if scheduled > 0 {
		res.AchievedRate = float64(res.Sent) / scheduled.Seconds()
	}
	if done := res.Successful + res.Failed; done > 0 {
		res.SuccessRate = float64(res.Successful) / float64(done) * 100
	}
	return res
}
