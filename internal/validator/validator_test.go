package validator

import (
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/stats"
)

// results returns complete-crawl results with response times 1..n ms.
func results(n int, success int64) *domain.TestResults {
	r := &domain.TestResults{
		TotalRequests:      int64(n),
		SuccessfulRequests: success,
		FailedRequests:     int64(n) - success,
		RequestsPerSecond:  10,
		CrawlComplete:      true,
	}
	r.ResponseTimes = stats.NewHistogram()
	for i := 1; i <= n; i++ {
		r.ResponseTimes.Record(time.Duration(i) * time.Millisecond)
	}
	return r
}

func judged(v *Validator) map[string]domain.PerformanceTarget {
	m := make(map[string]domain.PerformanceTarget, len(v.targets))
	for _, t := range v.targets {
		m[t.Name] = t
	}
	return m
}

func TestValidateResults_PercentileIsNearestRank(t *testing.T) {
	// p95 of 1..100ms is the 95th value, reported to within one histogram
	// bucket: in [95ms, 95ms*BucketRatio). A 94.9ms target fails; one at the
	// bucket's top passes.
	tests := []struct {
		targetMs float64
		pass     bool
	}{
		{94.9, false},
		{95 * stats.BucketRatio, true},
	}
	for _, tt := range tests {
		v := New(domain.PerformanceTargets{P95ResponseTimeMs: tt.targetMs})
		v.ValidateResults(results(100, 100))
		if got := judged(v)["95th Percentile Response Time"]; got.Passed != tt.pass {
			t.Errorf("p95 %s against %.2fms: passed %v, want %v", got.Actual, tt.targetMs, got.Passed, tt.pass)
		}
	}
}

func TestValidateResults_TargetCount(t *testing.T) {
	all := domain.PerformanceTargets{
		RequestsPerSecond: 1, AvgResponseTimeMs: 100, P95ResponseTimeMs: 100,
		P99ResponseTimeMs: 100, SuccessRate: 90,
	}
	p95Off := all
	p95Off.P95ResponseTimeMs = 0

	tests := []struct {
		name    string
		targets domain.PerformanceTargets
		want    []string
	}{
		{"defaults judge success rate only", domain.DefaultPerformanceTargets(), []string{"Success Rate"}},
		{"all enabled; RPS skipped without -load", all, []string{"Average Response Time", "95th Percentile Response Time",
			"99th Percentile Response Time", "Success Rate"}},
		{"a 0 target is not judged", p95Off, []string{"Average Response Time",
			"99th Percentile Response Time", "Success Rate"}},
		{"all disabled", domain.PerformanceTargets{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := New(tt.targets)
			v.ValidateResults(results(10, 10))
			var got []string
			for _, target := range v.targets {
				got = append(got, target.Name)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("judged %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPassed(t *testing.T) {
	targets := domain.PerformanceTargets{SuccessRate: 99, P95ResponseTimeMs: 100}
	incomplete := results(10, 10)
	incomplete.CrawlComplete = false
	incomplete.IncompleteReason = domain.IncompleteDuration

	tests := []struct {
		name    string
		results *domain.TestResults
		want    bool
	}{
		{"all targets met, crawl complete", results(10, 10), true},
		{"a target missed", results(10, 9), false},
		{"all targets met, crawl incomplete", incomplete, false},
		{"no requests", &domain.TestResults{CrawlComplete: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := New(targets)
			v.ValidateResults(tt.results)
			if got := v.Passed(); got != tt.want {
				t.Errorf("Passed() = %v, want %v", got, tt.want)
			}
			want := StatusFail
			if tt.want {
				want = StatusPass
			}
			if got := v.GetValidationSummary()["overall_status"]; got != want {
				t.Errorf("overall_status = %v, want %s", got, want)
			}
		})
	}
}

func TestPassed_NoTargetsCompleteCrawl(t *testing.T) {
	v := New(domain.PerformanceTargets{})
	v.ValidateResults(results(3, 3))
	if !v.Passed() {
		t.Error("a complete crawl with no enabled targets should pass")
	}
}

func TestValidateResults_ZeroRequests(t *testing.T) {
	v := New(domain.PerformanceTargets{SuccessRate: 99, AvgResponseTimeMs: 50})
	v.ValidateResults(&domain.TestResults{CrawlComplete: true})

	for _, target := range v.targets {
		if target.Passed || strings.Contains(target.Actual, "NaN") {
			t.Errorf("%s with zero requests: actual %q, passed %v; want a failure without NaN", target.Name, target.Actual, target.Passed)
		}
	}
	data, err := json.Marshal(v.GetValidationSummary())
	if err != nil {
		t.Fatalf("summary does not marshal: %v", err)
	}
	if strings.Contains(string(data), "NaN") {
		t.Errorf("summary contains NaN: %s", data)
	}
}

func TestGetValidationSummary_Structure(t *testing.T) {
	v := New(domain.PerformanceTargets{SuccessRate: 99})
	v.ValidateResults(results(10, 10))
	summary := v.GetValidationSummary()

	for _, key := range []string{"targets_met", "total_targets", "crawl_complete", "overall_status", "targets"} {
		if _, ok := summary[key]; !ok {
			t.Errorf("summary is missing %q", key)
		}
	}
	if summary["targets_met"] != 1 || summary["total_targets"] != 1 || summary["crawl_complete"] != true {
		t.Errorf("summary = %v", summary)
	}
}

func TestGetValidationSummary_NoTargets(t *testing.T) {
	v := New(domain.PerformanceTargets{})
	v.ValidateResults(&domain.TestResults{TotalRequests: 1, SuccessfulRequests: 1, CrawlComplete: true})

	if _, err := json.Marshal(v.GetValidationSummary()); err != nil {
		t.Fatalf("summary with no targets does not marshal: %v", err)
	}
}

func TestPrintValidationReport(t *testing.T) {
	// Covers the print paths for passing, failing and incomplete runs; output is not asserted.
	for _, r := range []*domain.TestResults{results(10, 10), results(10, 1), {IncompleteReason: domain.IncompleteQueueFull}} {
		v := New(domain.PerformanceTargets{SuccessRate: 99, RequestsPerSecond: math.SmallestNonzeroFloat64})
		v.ValidateResults(r)
		v.PrintValidationReport(io.Discard)
	}
}

func TestValidateResults_RPSSkippedWithoutLoad(t *testing.T) {
	v := New(domain.PerformanceTargets{RequestsPerSecond: 1000})
	v.ValidateResults(results(10, 10))
	if len(v.targets) != 0 || len(v.skipped) != 1 || !v.Passed() {
		t.Errorf("targets %v, skipped %v, passed %v; want RPS skipped and the run passing", v.targets, v.skipped, v.Passed())
	}
}

// loadResults returns load results with n latencies of 1..n ms, all successful.
func loadResults(n int, rate float64) *domain.LoadResults {
	h := stats.NewHistogram()
	for i := 1; i <= n; i++ {
		h.Record(time.Duration(i) * time.Millisecond)
	}
	return &domain.LoadResults{ResponseTimes: h, Sent: int64(n), Successful: int64(n), AchievedRate: rate}
}

func TestValidateResults_LoadPhase(t *testing.T) {
	targets := domain.PerformanceTargets{RequestsPerSecond: 50, P95ResponseTimeMs: 100, SuccessRate: 99}
	crawl := results(5, 5)
	crawl.Load = loadResults(100, 49)

	v := New(targets)
	v.ValidateResults(crawl)
	got := judged(v)
	for _, name := range []string{"Crawl Success Rate", "Load Requests per Second", "Load 95th Percentile Response Time", "Load Success Rate"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%q not judged; judged %v", name, got)
		}
	}
	if got["Load Requests per Second"].Passed || v.Passed() {
		t.Error("an achieved rate of 49 against a 50 req/s target should fail the run")
	}

	crawl.Load = loadResults(100, 50)
	crawl.Load.Aborted, crawl.Load.AbortReason = true, "10 consecutive 429/5xx responses"
	v.ValidateResults(crawl)
	if v.Passed() || v.GetValidationSummary()["load_aborted"] != true {
		t.Error("an aborted load should fail the run")
	}
}
