package domain

import (
	"time"

	"github.com/1mb-dev/lobster/v2/internal/stats"
)

// URLTask is a URL the crawl will request.
// Depth tracks how many links deep this URL was discovered from the base URL,
// used to enforce MaxDepth limits during recursive link discovery.
type URLTask struct {
	// URL is the fully-qualified URL to request.
	URL string
	// Depth is the crawl depth (0 = base URL, 1 = linked from base, etc.)
	Depth int
}

// TestResults contains a run's results: the crawl's, and the load's when -load ran.
// This is the main output structure containing all metrics, validations,
// and performance data collected during the test run.
type TestResults struct {
	// URLValidations contains individual validation results for each URL tested.
	URLValidations []URLValidation `json:"url_validations"`
	// Errors contains all errors encountered during testing.
	Errors []ErrorInfo `json:"errors"`
	// SlowRequests contains requests that exceeded the slow threshold (default 2s).
	SlowRequests []SlowRequest `json:"slow_requests"`
	// ResponseTimes counts response times in fixed buckets; memory does not grow with requests.
	ResponseTimes *stats.Histogram `json:"response_time_histogram"`
	// PerformanceValidation contains pass/fail status for each performance target.
	PerformanceValidation map[string]any `json:"performance_validation,omitempty"`
	// Duration is the total test execution time as a human-readable string.
	Duration string `json:"duration"`
	// AverageResponseTime is the mean response time as a human-readable string.
	AverageResponseTime string `json:"average_response_time"`
	// MinResponseTime is the fastest response time recorded.
	MinResponseTime string `json:"min_response_time"`
	// MaxResponseTime is the slowest response time recorded.
	MaxResponseTime string `json:"max_response_time"`
	// P95ResponseTime is the nearest-rank 95th percentile, to within one histogram bucket.
	P95ResponseTime string `json:"p95_response_time"`
	// P99ResponseTime is the nearest-rank 99th percentile, to within one histogram bucket.
	P99ResponseTime string `json:"p99_response_time"`
	// TotalRequests is the total number of HTTP requests made.
	TotalRequests int64 `json:"total_requests"`
	// SuccessfulRequests is the count of requests that got a 200-399 status.
	SuccessfulRequests int64 `json:"successful_requests"`
	// FailedRequests is the count of requests with a transport error or a status outside 200-399.
	FailedRequests int64 `json:"failed_requests"`
	// AbandonedRequests counts requests the -duration deadline cut off; they are not in TotalRequests.
	AbandonedRequests int64 `json:"abandoned_requests"`
	// RobotsBlocked counts discovered URLs robots.txt disallowed; they are not requested.
	RobotsBlocked int64 `json:"robots_blocked"`
	// URLsDropped counts discovered URLs the full queue could not take.
	URLsDropped int64 `json:"urls_dropped"`
	// CrawlComplete is true when every discovered URL was requested (none pending, dropped or abandoned) and no page's body was cut off while its links were read. Links are read from a page's first 10MB, and from none of a page that declares a larger Content-Length; what is not read does not make the crawl incomplete.
	CrawlComplete bool `json:"crawl_complete"`
	// IncompleteReason says why the crawl is incomplete: IncompleteQueueFull, IncompleteDuration or IncompleteLinksCutOff.
	IncompleteReason string `json:"incomplete_reason,omitempty"`
	// RequestsPerSecond is the average throughput during the test.
	RequestsPerSecond float64 `json:"requests_per_second"`
	// SuccessRate is the percentage of successful requests (0-100).
	SuccessRate float64 `json:"success_rate"`
	// Load holds the -load phase's results; nil when no load phase ran.
	Load *LoadResults `json:"load,omitempty"`
	// URLsDiscovered is the count of unique URLs found during link discovery.
	// Accessed atomically from multiple goroutines during crawl.
	URLsDiscovered int64 `json:"urls_discovered"`
}

// URLValidation represents the validation result for a single URL request.
// A request is considered valid if it completes without error and returns
// a successful HTTP status code (2xx or 3xx).
type URLValidation struct {
	// ResponseTime is how long the request took to complete.
	ResponseTime time.Duration `json:"response_time"`
	// ContentLength is the size of the response body in bytes.
	ContentLength int64 `json:"content_length"`
	// URL is the fully-qualified URL that was requested.
	URL string `json:"url"`
	// ContentType is the Content-Type header from the response.
	ContentType string `json:"content_type"`
	// Error contains the error message if the request failed, empty otherwise.
	Error string `json:"error,omitempty"`
	// StatusCode is the HTTP status code returned (0 if request failed).
	StatusCode int `json:"status_code"`
	// LinksFound is the count of valid links extracted from the response body.
	LinksFound int `json:"links_found"`
	// Depth is how deep in the crawl tree this URL was discovered.
	Depth int `json:"depth"`
	// IsValid is true if the request succeeded with a 2xx/3xx status.
	IsValid bool `json:"is_valid"`
}

// ErrorInfo is an error the crawl recorded for a URL.
// Errors are collected for reporting and debugging purposes.
type ErrorInfo struct {
	// Timestamp is when the error occurred.
	Timestamp time.Time `json:"timestamp"`
	// URL is the URL that caused the error.
	URL string `json:"url"`
	// Error is the error message (may be sanitized to hide internal details).
	Error string `json:"error"`
	// Depth is how deep in the crawl tree this URL was discovered.
	Depth int `json:"depth"`
}

// SlowRequest represents a request that exceeded the slow threshold (default 2s).
// These are tracked separately to help identify performance bottlenecks.
type SlowRequest struct {
	// ResponseTime is how long the slow request took.
	ResponseTime time.Duration `json:"response_time"`
	// URL is the slow endpoint.
	URL string `json:"url"`
	// StatusCode is the HTTP status returned.
	StatusCode int `json:"status_code"`
}

// PerformanceTarget represents the result of validating a performance criterion.
// Used in reports to show pass/fail status for each configured target.
type PerformanceTarget struct {
	// Name is a short identifier (e.g., "P95 Response Time").
	Name string
	// Target is the expected value as a string (e.g., "< 100ms").
	Target string
	// Actual is the measured value as a string.
	Actual string
	// Description explains what this target measures.
	Description string
	// Passed is true if the actual value met the target.
	Passed bool
}

// PerformanceTargets defines configurable performance criteria for pass/fail validation.
// Tests are evaluated against these targets to determine overall success.
type PerformanceTargets struct {
	// RequestsPerSecond is the minimum acceptable throughput.
	RequestsPerSecond float64 `json:"requests_per_second"`
	// AvgResponseTimeMs is the maximum acceptable average response time in milliseconds.
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
	// P95ResponseTimeMs is the maximum acceptable 95th percentile response time.
	P95ResponseTimeMs float64 `json:"p95_response_time_ms"`
	// P99ResponseTimeMs is the maximum acceptable 99th percentile response time.
	P99ResponseTimeMs float64 `json:"p99_response_time_ms"`
	// SuccessRate is the minimum acceptable success percentage (0-100).
	SuccessRate float64 `json:"success_rate"`
}

// DefaultPerformanceTargets enables only the success-rate target; a target of 0 is disabled.
func DefaultPerformanceTargets() PerformanceTargets {
	return PerformanceTargets{SuccessRate: 99.0}
}

// IncompleteReason values.
const (
	IncompleteQueueFull   = "queue_full"
	IncompleteDuration    = "duration"
	IncompleteLinksCutOff = "links_cut_off"
)

// AddURLResult constants for URL addition outcomes.
const (
	AddURLSuccess       = "success"
	AddURLDuplicate     = "duplicate"
	AddURLQueueFull     = "queue_full"
	AddURLDepthExceeded = "depth_exceeded"
	AddURLInvalidHost   = "invalid_host"
	AddURLParseError    = "parse_error"
)

// AddURLResult represents the result of attempting to add a URL to the crawl queue.
// Provides detailed feedback about why a URL was or wasn't added.
type AddURLResult struct {
	// Added is true if the URL was successfully added to the queue.
	Added bool
	// Reason explains why the URL was or wasn't added.
	// Values: "success", "duplicate", "queue_full", "depth_exceeded", "invalid_host", "parse_error"
	Reason string
}

// LoadResults are the results of the -load phase: the crawled URLs replayed
// open-loop at the target rate.
type LoadResults struct {
	// ResponseTimes counts latencies measured from each request's intended send time.
	ResponseTimes *stats.Histogram `json:"response_time_histogram"`
	// Duration is how long requests were scheduled for.
	Duration string `json:"duration"`
	// AbortReason says why the load stopped early; empty when it ran to the end.
	AbortReason string `json:"abort_reason,omitempty"`
	// Average, P95 and P99 summarize ResponseTimes.
	AverageResponseTime string `json:"average_response_time"`
	P95ResponseTime     string `json:"p95_response_time"`
	P99ResponseTime     string `json:"p99_response_time"`
	// TargetRate is the requested rate (-rate), in requests per second.
	TargetRate float64 `json:"target_rate"`
	// AchievedRate is requests sent divided by Duration.
	AchievedRate float64 `json:"achieved_rate"`
	// SuccessRate is the percentage of finished requests (successful plus failed) that got a 200-399 status.
	SuccessRate float64 `json:"success_rate"`
	// Sent counts requests sent.
	Sent int64 `json:"requests_sent"`
	// Successful counts requests that got a 200-399 status.
	Successful int64 `json:"successful_requests"`
	// Failed counts requests with a transport error or a status outside 200-399.
	Failed int64 `json:"failed_requests"`
	// Late counts requests that waited for an in-flight slot, and those after such a request due before its slot came free.
	Late int64 `json:"late"`
	// Unsent counts requests due in the window that were not sent: delayed by a wait for a slot, or finding no slot free, when it ended, or cut off by an abort.
	Unsent int64 `json:"requests_unsent"`
	// Aborted is true when consecutive 429, 5xx or connection errors stopped the load.
	Aborted bool `json:"load_aborted"`
}
