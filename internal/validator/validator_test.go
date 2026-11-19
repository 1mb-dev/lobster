package validator

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
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
	for i := 1; i <= n; i++ {
		r.ResponseTimes = append(r.ResponseTimes, domain.ResponseTimeEntry{ResponseTime: time.Duration(i) * time.Millisecond})
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
	v := New(domain.PerformanceTargets{P95ResponseTimeMs: 95, P99ResponseTimeMs: 98.5})
	v.ValidateResults(results(100, 100))

	got := judged(v)
	if p95 := got["95th Percentile Response Time"]; p95.Actual != "95.0ms" || !p95.Passed {
		t.Errorf("p95 of 1..100ms = %s (passed %v), want the 95th value, 95.0ms, passing a 95ms target", p95.Actual, p95.Passed)
	}
	if p99 := got["99th Percentile Response Time"]; p99.Actual != "99.0ms" || p99.Passed {
		t.Errorf("p99 of 1..100ms = %s (passed %v), want 99.0ms, failing a 98.5ms target", p99.Actual, p99.Passed)
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
		{"all enabled", all, []string{"Requests per Second", "Average Response Time", "95th Percentile Response Time",
			"99th Percentile Response Time", "Success Rate"}},
		{"a 0 target is not judged", p95Off, []string{"Requests per Second", "Average Response Time",
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
		v.PrintValidationReport()
	}
}
