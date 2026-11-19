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
	// AbortAfter is how many consecutive 429/5xx responses stop the load.
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

	abortOnce   sync.Once
	abortReason string
	cancel      context.CancelFunc
}

// record adds one finished request: its latency from the intended send time
// and its status (0 for a transport error). It does not allocate.
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
	if status == http.StatusTooManyRequests || status >= 500 {
		if r.consec.Add(1) >= AbortAfter {
			r.abort(fmt.Sprintf("%d consecutive 429/5xx responses", AbortAfter))
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
// are busy, a request waits for one and is counted late; once a slot frees,
// requests already due go out at once.
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
	timer := time.NewTimer(0)
	defer timer.Stop()
	var lastSend time.Time

	for i := 0; ; i++ {
		// Request i is due at start + i/rate whatever happened before it: a
		// token bucket would drop the sends missed while waiting for a slot,
		// hiding the backlog (coordinated omission).
		intended := start.Add(time.Duration(float64(i) * float64(time.Second) / cfg.Rate))
		if !intended.Before(end) {
			break
		}
		if !sleepUntil(ctx, timer, intended) {
			break
		}
		select {
		case slots <- struct{}{}:
		default:
			r.late.Add(1)
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			break
		}

		lastSend = time.Now()
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
		}(cfg.URLs[i%len(cfg.URLs)], intended)
	}
	stopped := time.Since(start)
	wg.Wait() // abortReason is written by request goroutines; read it after they finish

	// On schedule, the last send is due before the end; a sender that fell
	// behind sends later, and the achieved rate shows it.
	window := cfg.Duration
	if r.abortReason != "" {
		window = stopped
	} else if behind := lastSend.Sub(start); behind > window {
		window = behind
	}

	return r.results(cfg, window), nil
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

// send performs one request and returns its status, or 0 on a transport error.
func send(ctx context.Context, cfg Config, rawURL string) int {
	req, err := cfg.Requester.NewRequest(ctx, rawURL)
	if err != nil {
		return 0
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
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
		Aborted:             r.abortReason != "",
	}
	if scheduled > 0 {
		res.AchievedRate = float64(res.Sent) / scheduled.Seconds()
	}
	if res.Sent > 0 {
		res.SuccessRate = float64(res.Successful) / float64(res.Sent) * 100
	}
	return res
}
