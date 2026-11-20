// Package testutil provides shared test fixtures and utilities for use across test files.
package testutil

import (
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/stats"
)

// SampleResults returns a complete test results fixture for use in tests.
// This is the standard fixture used across multiple test files.
func SampleResults() *domain.TestResults {
	return &domain.TestResults{
		Duration:            "2m30s",
		URLsDiscovered:      10,
		TotalRequests:       100,
		SuccessfulRequests:  95,
		FailedRequests:      5,
		AverageResponseTime: "150ms",
		MinResponseTime:     "50ms",
		MaxResponseTime:     "500ms",
		RequestsPerSecond:   0.67,
		SuccessRate:         95.0,
		URLValidations: []domain.URLValidation{
			{
				URL:           "http://example.com",
				StatusCode:    200,
				ResponseTime:  100 * time.Millisecond,
				ContentLength: 1024,
				ContentType:   "text/html",
				LinksFound:    5,
				Depth:         0,
				IsValid:       true,
			},
			{
				URL:           "http://example.com/404",
				StatusCode:    404,
				ResponseTime:  50 * time.Millisecond,
				ContentLength: 0,
				ContentType:   "text/html",
				LinksFound:    0,
				Depth:         1,
				IsValid:       false,
			},
		},
		Errors: []domain.ErrorInfo{
			{
				URL:       "http://example.com/error",
				Error:     "connection timeout",
				Timestamp: time.Now(),
				Depth:     1,
			},
		},
		SlowRequests: []domain.SlowRequest{
			{
				URL:          "http://example.com/slow",
				ResponseTime: 3 * time.Second,
				StatusCode:   200,
			},
		},
		ResponseTimes: Histogram(100*time.Millisecond, 50*time.Millisecond),
	}
}

// MinimalResults returns a minimal test results fixture with only required fields.
func MinimalResults() *domain.TestResults {
	return &domain.TestResults{
		Duration:           "1m0s",
		URLsDiscovered:     1,
		TotalRequests:      10,
		SuccessfulRequests: 10,
		FailedRequests:     0,
		RequestsPerSecond:  1.0,
		SuccessRate:        100.0,
		URLValidations:     []domain.URLValidation{},
		Errors:             []domain.ErrorInfo{},
		SlowRequests:       []domain.SlowRequest{},
		ResponseTimes:      Histogram(),
	}
}

// EmptyResults returns an empty test results fixture for testing edge cases.
func EmptyResults() *domain.TestResults {
	return &domain.TestResults{
		URLValidations: []domain.URLValidation{},
		Errors:         []domain.ErrorInfo{},
		SlowRequests:   []domain.SlowRequest{},
		ResponseTimes:  Histogram(),
	}
}

// Histogram returns a histogram holding durations.
func Histogram(durations ...time.Duration) *stats.Histogram {
	h := stats.NewHistogram()
	for _, d := range durations {
		h.Record(d)
	}
	return h
}
