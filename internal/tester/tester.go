// Package tester implements the core load testing engine with concurrent workers.
package tester

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/1mb-dev/goflow/pkg/ratelimit/bucket"
	"github.com/1mb-dev/lobster/v2/internal/crawler"
	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/robots"
	"github.com/1mb-dev/lobster/v2/internal/stats"
	"github.com/1mb-dev/lobster/v2/internal/util"
)

const (
	// defaultSlowRequestThreshold is the response time above which requests are flagged as slow.
	defaultSlowRequestThreshold = 2 * time.Second

	// linkExtractionLimit is the maximum bytes to read for link discovery.
	// 64KB is sufficient for most pages as links are typically in the head/upper body.
	linkExtractionLimit = 64 * 1024

	// defaultQueueSize is the default URL queue capacity when not configured.
	defaultQueueSize = 10000
)

// Tester orchestrates the stress testing process
type Tester struct {
	config       domain.TesterConfig
	client       *http.Client
	urlQueue     chan domain.URLTask
	results      *domain.TestResults
	rateLimiter  domain.RateLimiter
	crawler      domain.URLCrawler
	robotsParser domain.RobotsChecker
	logger       *slog.Logger

	// Result channels for lock-free aggregation
	validationsCh   chan domain.URLValidation
	errorsCh        chan domain.ErrorInfo
	responseTimesCh chan time.Duration
	slowRequestsCh  chan domain.SlowRequest

	crawlDelay time.Duration

	// baseRefused is set by the base URL's task when the dial check or the
	// redirect policy refuses it; read after the workers stop.
	baseRefused error

	pending  atomic.Int64 // tasks enqueued and not yet finished
	done     chan struct{}
	doneOnce sync.Once
}

// New creates a new stress tester
func New(config domain.TesterConfig, logger *slog.Logger) (*Tester, error) {
	// Create crawler
	crawlerInstance, err := crawler.New(config.BaseURL, config.MaxDepth)
	if err != nil {
		return nil, fmt.Errorf("creating crawler: %w", err)
	}

	if config.Rate <= 0 {
		return nil, fmt.Errorf("rate must be > 0, got %g", config.Rate)
	}

	// Use configured queue size, default to defaultQueueSize if not set
	queueSize := config.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	if config.InsecureSkipVerify {
		logger.Warn("INSECURE: TLS certificate verification is disabled. Use only for testing with self-signed certificates!")
	}
	transport := newTransport(config, config.Concurrency*2, util.DialControl(config.AllowPrivateIPs))

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}
	robotsParser := robots.New(config.UserAgent)

	// Create HTTP client with connection pooling
	httpClient := &http.Client{
		Timeout:       config.RequestTimeout,
		Transport:     transport,
		CheckRedirect: redirectPolicy(baseURL, robotsParser, config.IgnoreRobots),
	}

	// robots.txt is fetched through the same transport, without the robots recheck.
	robotsParser.SetClient(&http.Client{
		Timeout:       10 * time.Second,
		Transport:     transport,
		CheckRedirect: redirectPolicy(baseURL, robotsParser, true),
	})
	if !config.IgnoreRobots {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if fetchErr := robotsParser.FetchAndParse(ctx, config.BaseURL); fetchErr != nil {
			logger.Warn("Failed to fetch robots.txt, proceeding with caution", "error", fetchErr)
		} else if robotsParser.RobotsTxtFound() {
			logger.Info("robots.txt found and parsed successfully")
		} else {
			logger.Debug("No robots.txt found, all paths allowed")
		}
	} else {
		logger.Warn("WARNING: Ignoring robots.txt directives. Please ensure you have permission to test this site!")
	}

	rate := effectiveRate(config.Rate, robotsParser.GetCrawlDelay())
	if rate < config.Rate {
		logger.Info("robots.txt Crawl-delay lowers the rate",
			"crawl_delay", robotsParser.GetCrawlDelay(), "rate", rate)
	}
	// Burst 1: requests are spaced at 1/rate from the first one.
	rateLimiter, err := bucket.NewSafe(bucket.Limit(rate), 1)
	if err != nil {
		return nil, fmt.Errorf("creating rate limiter (rate=%.2f): %w", rate, err)
	}

	// Size result channels proportionally to avoid backpressure
	// Use larger buffers when queue is large to handle burst processing
	resultBufferSize := min(queueSize, 10000)
	slowBufferSize := min(queueSize/10, 1000)

	return &Tester{
		config:          config,
		crawlDelay:      robotsParser.GetCrawlDelay(),
		client:          httpClient,
		urlQueue:        make(chan domain.URLTask, queueSize),
		results:         &domain.TestResults{URLValidations: make([]domain.URLValidation, 0)},
		rateLimiter:     rateLimiter,
		crawler:         crawlerInstance,
		robotsParser:    robotsParser,
		logger:          logger,
		validationsCh:   make(chan domain.URLValidation, resultBufferSize),
		errorsCh:        make(chan domain.ErrorInfo, resultBufferSize),
		responseTimesCh: make(chan time.Duration, resultBufferSize),
		slowRequestsCh:  make(chan domain.SlowRequest, slowBufferSize),
		done:            make(chan struct{}),
	}, nil
}

// Run executes the stress test
func (t *Tester) Run(ctx context.Context) (*domain.TestResults, error) {
	startTime := time.Now()

	// Initialize results
	t.results.ResponseTimes = stats.NewHistogram()
	t.results.Errors = make([]domain.ErrorInfo, 0)
	t.results.SlowRequests = make([]domain.SlowRequest, 0)

	var wg sync.WaitGroup
	var aggregatorWg sync.WaitGroup

	// Start result aggregator
	aggregatorWg.Add(1)
	go t.aggregator(&aggregatorWg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start workers
	for i := 0; i < t.config.Concurrency; i++ {
		wg.Add(1)
		go t.worker(ctx, &wg)
	}

	// Start URL discovery with the base URL
	t.enqueue(t.config.BaseURL, 0)
	atomic.StoreInt64(&t.results.URLsDiscovered, int64(t.crawler.GetDiscoveredCount()))

	// Start monitoring
	go t.monitor(ctx, startTime)

	select {
	case <-ctx.Done():
	case <-t.done:
	}
	cancel()
	wg.Wait()

	// Close result channels and wait for aggregator to finish
	close(t.validationsCh)
	close(t.errorsCh)
	close(t.responseTimesCh)
	close(t.slowRequestsCh)
	aggregatorWg.Wait()

	if t.baseRefused != nil {
		return nil, fmt.Errorf("base URL refused: %w", t.baseRefused)
	}
	t.recordCompleteness()

	// Calculate final results
	t.calculateResults(time.Since(startTime))

	return t.results, nil
}

// aggregator collects results from workers via channels (lock-free).
// Uses nil channel pattern: closed channels are set to nil to disable their select cases.
func (t *Tester) aggregator(wg *sync.WaitGroup) {
	defer wg.Done()

	// Local channel references - set to nil when closed to disable select case
	validationsCh := t.validationsCh
	errorsCh := t.errorsCh
	responseTimesCh := t.responseTimesCh
	slowRequestsCh := t.slowRequestsCh

	// Continue while any channel is still open
	for validationsCh != nil || errorsCh != nil || responseTimesCh != nil || slowRequestsCh != nil {
		select {
		case validation, ok := <-validationsCh:
			if !ok {
				validationsCh = nil
				continue
			}
			t.results.URLValidations = append(t.results.URLValidations, validation)

		case errInfo, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			t.results.Errors = append(t.results.Errors, errInfo)

		case responseTime, ok := <-responseTimesCh:
			if !ok {
				responseTimesCh = nil
				continue
			}
			t.results.ResponseTimes.Record(responseTime)

		case slowReq, ok := <-slowRequestsCh:
			if !ok {
				slowRequestsCh = nil
				continue
			}
			t.results.SlowRequests = append(t.results.SlowRequests, slowReq)
		}
	}
}

// worker processes URLs from the queue
func (t *Tester) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case task := <-t.urlQueue:
			t.processURL(ctx, task)
			t.finish()
		case <-ctx.Done():
			return
		}
	}
}

// processURL performs a single URL request and records results
func (t *Tester) processURL(ctx context.Context, task domain.URLTask) {
	// Check robots.txt compliance (unless ignoring)
	if !t.config.IgnoreRobots && !t.robotsParser.IsAllowed(task.URL) {
		t.logger.Debug("URL blocked by robots.txt", "url", util.SanitizeURLDefault(task.URL))
		atomic.AddInt64(&t.results.RobotsBlocked, 1)
		return
	}

	// A wait or request the deadline cuts off touches no request counter.
	if err := t.rateLimiter.Wait(ctx); err != nil {
		atomic.AddInt64(&t.results.AbandonedRequests, 1)
		return
	}

	// Make HTTP request with 429 retry logic
	resp, responseTime, err := t.makeHTTPRequestWithRetry(ctx, task.URL)
	if err != nil {
		if ctx.Err() != nil {
			atomic.AddInt64(&t.results.AbandonedRequests, 1)
			return
		}
		if refused := refusal(err); refused != nil {
			if task.Depth == 0 {
				// A refused base URL ends the run with an error, not a failed page.
				t.baseRefused = refused
				return
			}
			err = refused
		}
		atomic.AddInt64(&t.results.TotalRequests, 1)
		atomic.AddInt64(&t.results.FailedRequests, 1)
		t.recordError(task.URL, fmt.Sprintf("making request: %v", err), task.Depth)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	atomic.AddInt64(&t.results.TotalRequests, 1)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	if ok {
		atomic.AddInt64(&t.results.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&t.results.FailedRequests, 1)
		t.recordError(task.URL, fmt.Sprintf("HTTP %d", resp.StatusCode), task.Depth)
	}

	// Record response time
	t.responseTimesCh <- responseTime

	// Create validation record
	validation := domain.URLValidation{
		URL:           task.URL,
		StatusCode:    resp.StatusCode,
		ResponseTime:  responseTime,
		ContentLength: resp.ContentLength,
		ContentType:   resp.Header.Get("Content-Type"),
		Depth:         task.Depth,
		IsValid:       ok,
	}

	// Discover links if configured
	validation.LinksFound = t.discoverLinksFromResponse(resp, task)

	// Record slow requests exceeding threshold
	if responseTime > defaultSlowRequestThreshold {
		t.recordSlowRequest(task.URL, responseTime, resp.StatusCode)
	}

	// Add validation to results (thread-safe)
	t.addValidation(validation)

	t.logger.Debug("URL processed",
		"url", util.SanitizeURLDefault(task.URL),
		"status", resp.StatusCode,
		"response_time", responseTime,
		"depth", task.Depth,
		"links_found", validation.LinksFound)
}

// makeHTTPRequestWithRetry wraps makeHTTPRequest with exponential backoff retry for 429 responses.
// Returns the actual request duration (excluding backoff time) for accurate latency metrics.
func (t *Tester) makeHTTPRequestWithRetry(ctx context.Context, rawURL string) (*http.Response, time.Duration, error) {
	const (
		maxRetries     = 4 // Max retry attempts for 429
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)

	// Track actual request time separately from backoff time
	// Only request time is used for latency metrics
	var lastRequestDuration time.Duration
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, duration, err := t.makeHTTPRequest(ctx, rawURL)
		lastRequestDuration = duration

		// If request failed (network error, etc), return error immediately
		if err != nil {
			return nil, lastRequestDuration, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, lastRequestDuration, nil
		}

		// Close the 429 response body before retrying
		_ = resp.Body.Close()

		// If this was the last attempt, return the 429 response
		if attempt == maxRetries {
			// Re-make request one final time to return a valid response object
			return t.makeHTTPRequest(ctx, rawURL)
		}

		// Log the backoff
		t.logger.Info("Received 429 Too Many Requests, backing off",
			"rawURL", util.SanitizeURLDefault(rawURL),
			"attempt", attempt+1,
			"backoff", backoff,
			"max_retries", maxRetries)

		// Wait for backoff period or context cancellation
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return nil, lastRequestDuration, ctx.Err()
		}

		// Exponential backoff: double each time, cap at maxBackoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// Should never reach here, but return error just in case
	return nil, lastRequestDuration, fmt.Errorf("exceeded max retries for 429")
}

// makeHTTPRequest creates and executes an HTTP request, returning the response and duration
func (t *Tester) makeHTTPRequest(ctx context.Context, rawURL string) (*http.Response, time.Duration, error) {
	startTime := time.Now()

	req, err := t.NewRequest(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}

	// Execute request
	resp, err := t.client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		return nil, responseTime, err
	}

	return resp, responseTime, nil
}

// applyAuthentication applies configured authentication to the HTTP request
func (t *Tester) applyAuthentication(req *http.Request) error {
	if t.config.Auth == nil {
		return nil
	}

	auth := t.config.Auth

	switch auth.Type {
	case "basic":
		// Basic authentication: username:password
		if auth.Username != "" {
			req.SetBasicAuth(auth.Username, auth.Password)
			t.logger.Debug("Applied basic authentication")
		}

	case "bearer":
		// Bearer token authentication
		if auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+auth.Token)
			t.logger.Debug("Applied bearer token authentication")
		}

	case "cookie":
		// Cookie-based authentication
		for name, value := range auth.Cookies {
			req.AddCookie(&http.Cookie{
				Name:  name,
				Value: value,
			})
			t.logger.Debug("Applied cookie", "name", name)
		}

	case "header":
		// Custom header-based authentication
		for name, value := range auth.Headers {
			req.Header.Set(name, value)
			t.logger.Debug("Applied custom header", "name", name)
		}

	case "":
		// No authentication type specified, check for individual fields
		if auth.Username != "" {
			req.SetBasicAuth(auth.Username, auth.Password)
		} else if auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+auth.Token)
		}
		if len(auth.Cookies) > 0 {
			for name, value := range auth.Cookies {
				req.AddCookie(&http.Cookie{
					Name:  name,
					Value: value,
				})
			}
		}
		if len(auth.Headers) > 0 {
			for name, value := range auth.Headers {
				req.Header.Set(name, value)
			}
		}

	default:
		return fmt.Errorf("unknown authentication type: %s", auth.Type)
	}

	return nil
}

// discoverLinksFromResponse extracts links from HTML responses and adds them to the crawl queue
func (t *Tester) discoverLinksFromResponse(resp *http.Response, task domain.URLTask) int {
	// Only process HTML responses
	if task.Depth >= t.config.MaxDepth ||
		!strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return 0
	}

	// Check Content-Length before reading body
	maxSize := t.config.MaxResponseSize
	if maxSize == 0 {
		maxSize = 10 * 1024 * 1024 // Default 10MB
	}
	if resp.ContentLength > maxSize {
		t.logger.Debug("Skipping link extraction: response too large",
			"url", util.SanitizeURLDefault(task.URL),
			"content_length", resp.ContentLength,
			"max_size", maxSize)
		return 0
	}

	// Limit body reading for link extraction (smaller than MaxResponseSize)
	limitedReader := io.LimitReader(resp.Body, linkExtractionLimit)
	body, readErr := io.ReadAll(limitedReader)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		t.logger.Debug("Error reading response body for link extraction",
			"url", util.SanitizeURLDefault(task.URL),
			"error", readErr)
		return 0
	}

	// Extract and queue links
	links := t.crawler.ExtractLinks(string(body))
	for _, link := range links {
		if t.enqueue(link, task.Depth+1) {
			atomic.StoreInt64(&t.results.URLsDiscovered, int64(t.crawler.GetDiscoveredCount()))
		}
	}

	return len(links)
}

// recordError records an error encountered during testing.
// Error messages are sanitized to hide internal infrastructure details
// unless verbose mode is enabled.
func (t *Tester) recordError(rawURL, errMsg string, depth int) {
	// Sanitize error message to hide internal details unless verbose
	sanitizedErr := util.SanitizeErrorForDisplay(errMsg, t.config.Verbose)

	errorInfo := domain.ErrorInfo{
		URL:       rawURL,
		Error:     sanitizedErr,
		Timestamp: time.Now(),
		Depth:     depth,
	}
	t.addError(errorInfo)
}

// recordSlowRequest records a slow request
func (t *Tester) recordSlowRequest(rawURL string, responseTime time.Duration, statusCode int) {
	slowReq := domain.SlowRequest{
		URL:          rawURL,
		ResponseTime: responseTime,
		StatusCode:   statusCode,
	}
	t.addSlowRequest(slowReq)
}

// Lock-free methods for sending results to aggregator via channels
func (t *Tester) addValidation(validation domain.URLValidation) {
	t.validationsCh <- validation
}

func (t *Tester) addError(errInfo domain.ErrorInfo) {
	t.errorsCh <- errInfo
}

func (t *Tester) addSlowRequest(req domain.SlowRequest) {
	t.slowRequestsCh <- req
}

// monitor provides real-time progress updates
func (t *Tester) monitor(ctx context.Context, startTime time.Time) {
	// Recover from panics to prevent monitor failure from crashing the test
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "\nMonitor panic recovered: %v\n", r)
		}
	}()

	if t.config.NoProgress {
		// No progress updates, just wait for context
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Safely load atomic counters
			total := atomic.LoadInt64(&t.results.TotalRequests)
			successful := atomic.LoadInt64(&t.results.SuccessfulRequests)
			failed := atomic.LoadInt64(&t.results.FailedRequests)
			discovered := atomic.LoadInt64(&t.results.URLsDiscovered)
			queueSize := len(t.urlQueue)

			// Calculate elapsed time and rate
			elapsed := time.Since(startTime)
			reqPerSec := float64(total) / elapsed.Seconds()

			if t.config.Verbose {
				// Verbose mode: use structured logging
				if t.logger != nil {
					t.logger.Info("Progress update",
						"elapsed", elapsed.Round(time.Second).String(),
						"total_requests", total,
						"successful", successful,
						"failed", failed,
						"urls_discovered", discovered,
						"queue_size", queueSize,
						"req_per_sec", fmt.Sprintf("%.1f", reqPerSec))
				}
			} else {
				// Normal mode: clean progress bar (overwrite same line)
				fmt.Fprintf(os.Stderr, "\rElapsed: %s | Requests: %d (%.1f/s) | Success: %d | Failed: %d | URLs: %d | Queue: %d",
					elapsed.Round(time.Second),
					total,
					reqPerSec,
					successful,
					failed,
					discovered,
					queueSize)
			}
		case <-ctx.Done():
			if !t.config.Verbose {
				// Print newline to finalize progress line
				fmt.Fprintf(os.Stderr, "\n")
			}
			return
		}
	}
}

// calculateResults computes final statistics
// Note: Safe to access results directly since aggregator has finished
func (t *Tester) calculateResults(duration time.Duration) {
	t.results.Duration = duration.String()

	if summary := t.results.ResponseTimes.Summary(); summary.Count > 0 {
		t.results.MinResponseTime = summary.Min.String()
		t.results.MaxResponseTime = summary.Max.String()
		t.results.AverageResponseTime = summary.Mean.String()
		t.results.P95ResponseTime = summary.P95.String()
		t.results.P99ResponseTime = summary.P99.String()
	}

	// Calculate rates
	if duration.Seconds() > 0 {
		t.results.RequestsPerSecond = float64(t.results.TotalRequests) / duration.Seconds()
	}

	if t.results.TotalRequests > 0 {
		t.results.SuccessRate = (float64(t.results.SuccessfulRequests) / float64(t.results.TotalRequests)) * 100
	}

	// Sort slow requests by response time
	sort.Slice(t.results.SlowRequests, func(i, j int) bool {
		return t.results.SlowRequests[i].ResponseTime > t.results.SlowRequests[j].ResponseTime
	})
}

// enqueue counts a task as pending before it becomes visible to workers.
func (t *Tester) enqueue(rawURL string, depth int) bool {
	t.pending.Add(1)
	if t.crawler.AddURL(rawURL, depth, t.urlQueue).Added {
		return true
	}
	t.finish()
	return false
}

// finish marks one task done; the crawl is complete when none are pending.
func (t *Tester) finish() {
	if t.pending.Add(-1) == 0 {
		t.doneOnce.Do(func() { close(t.done) })
	}
}

// recordCompleteness sets CrawlComplete and IncompleteReason once workers have stopped.
func (t *Tester) recordCompleteness() {
	t.results.URLsDropped = int64(t.crawler.GetDroppedCount())
	switch {
	case t.results.URLsDropped > 0:
		t.results.IncompleteReason = domain.IncompleteQueueFull
		t.logger.Warn("URLs dropped due to queue overflow",
			"dropped_count", t.results.URLsDropped,
			"hint", "Increase -queue-size")
	case t.pending.Load() > 0 || t.results.AbandonedRequests > 0:
		t.results.IncompleteReason = domain.IncompleteDuration
	default:
		t.results.CrawlComplete = true
	}
}

// effectiveRate applies robots.txt Crawl-delay as a ceiling: min(rate, 1/delay).
func effectiveRate(rate float64, crawlDelay time.Duration) float64 {
	if crawlDelay <= 0 {
		return rate
	}
	return min(rate, 1/crawlDelay.Seconds())
}

// newTransport builds the HTTP transport every request goes through: control
// checks each dialed address, there is no proxy (a proxy would dial the target
// itself, bypassing the check), and maxConns connections go to the host.
func newTransport(config domain.TesterConfig, maxConns int, control func(string, string, syscall.RawConn) error) *http.Transport {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   control,
		}).DialContext,
		MaxIdleConns:        max(100, maxConns),
		MaxIdleConnsPerHost: maxConns,
		MaxConnsPerHost:     maxConns,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if config.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Intentionally insecure for testing self-signed certs
		}
	}
	return transport
}

// NewRequest builds a GET for rawURL with the run's user agent and authentication.
func (t *Tester) NewRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", t.config.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err := t.applyAuthentication(req); err != nil {
		return nil, fmt.Errorf("applying authentication: %w", err)
	}
	return req, nil
}

// LoadClient returns a client with the crawl's dial check, TLS settings and
// redirect policy, sized for maxConns concurrent connections. With
// privateOnly it also refuses every public address: both checks must pass.
func (t *Tester) LoadClient(maxConns int, privateOnly bool) *http.Client {
	control := util.DialControl(t.config.AllowPrivateIPs)
	if privateOnly {
		crawlCheck, privateCheck := control, util.PrivateOnlyDialControl()
		control = func(network, address string, c syscall.RawConn) error {
			if err := crawlCheck(network, address, c); err != nil {
				return err
			}
			return privateCheck(network, address, c)
		}
	}
	return &http.Client{
		Timeout:       t.config.RequestTimeout,
		Transport:     newTransport(t.config, maxConns, control),
		CheckRedirect: t.client.CheckRedirect,
	}
}

// CrawlDelay returns the robots.txt Crawl-delay; 0 when there is none or robots.txt is ignored.
func (t *Tester) CrawlDelay() time.Duration {
	return t.crawlDelay
}
