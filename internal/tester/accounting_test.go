package tester

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
)

// site serves HTML pages from a path→links map; unknown paths get 404.
// robots, when non-empty, is served at /robots.txt; redirects maps a path to
// its 301 target. hits counts page requests.
type site struct {
	pages     map[string][]string
	redirects map[string]string
	robots    string
	delay     time.Duration

	mu   sync.Mutex
	hits map[string]int
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/robots.txt" {
		if s.robots == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(s.robots))
		return
	}
	if to, ok := s.redirects[r.URL.Path]; ok {
		http.Redirect(w, r, to, http.StatusMovedPermanently)
		return
	}
	s.mu.Lock()
	s.hits[r.URL.Path]++
	s.mu.Unlock()
	links, ok := s.pages[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	time.Sleep(s.delay)
	w.Header().Set("Content-Type", "text/html")
	for _, l := range links {
		_, _ = fmt.Fprintf(w, `<a href="%s">x</a>`, l)
	}
}

func (s *site) pageHits() (total int, byPath map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byPath = make(map[string]int, len(s.hits))
	for p, n := range s.hits {
		byPath[p] = n
		total += n
	}
	return total, byPath
}

// crawl runs a tester against s, starting at base+path, with mutate applied to the config.
func crawl(t *testing.T, s *site, path string, ceiling time.Duration, mutate func(*domain.TesterConfig)) *domain.TestResults {
	t.Helper()
	s.hits = map[string]int{}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)

	config := testConfig(server.URL + path)
	config.MaxDepth = 3
	config.NoProgress = true
	if mutate != nil {
		mutate(&config)
	}
	tester, err := New(config, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ceiling)
	defer cancel()
	results, err := tester.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return results
}

func TestAccounting_NotFoundIsFailure(t *testing.T) {
	s := &site{pages: map[string][]string{"/": {"/missing1", "/missing2"}}}
	r := crawl(t, s, "/", 10*time.Second, nil)

	if r.TotalRequests != 3 || r.SuccessfulRequests != 1 || r.FailedRequests != 2 {
		t.Errorf("total/success/failed = %d/%d/%d, want 3/1/2", r.TotalRequests, r.SuccessfulRequests, r.FailedRequests)
	}
	if r.SuccessRate >= 100 {
		t.Errorf("SuccessRate = %.2f, want below 100 with two 404s", r.SuccessRate)
	}
}

func TestAccounting_BaseURLFetchedOnce(t *testing.T) {
	s := &site{pages: map[string][]string{"/": {"/", "/a"}, "/a": {"/"}}}
	crawl(t, s, "", 10*time.Second, nil) // base URL with an empty path

	if _, hits := s.pageHits(); hits["/"] != 1 {
		t.Errorf("base page fetched %d times, want 1", hits["/"])
	}
}

// A relative link resolves against the page it was found on, after redirects.
func TestCrawl_RelativeLinkResolvesAgainstPage(t *testing.T) {
	s := &site{
		pages: map[string][]string{
			"/":                 {"/docs/index.html", "/guide"},
			"/docs/index.html":  {"intro.html"},
			"/docs/intro.html":  nil,
			"/guide/":           {"start.html"},
			"/guide/start.html": nil,
		},
		redirects: map[string]string{"/guide": "/guide/"},
	}
	r := crawl(t, s, "/", 10*time.Second, nil)

	_, hits := s.pageHits()
	for _, p := range []string{"/docs/intro.html", "/guide/start.html"} {
		if hits[p] != 1 {
			t.Errorf("%s fetched %d times, want 1 (pages fetched: %v)", p, hits[p], hits)
		}
	}
	if r.FailedRequests != 0 {
		t.Errorf("%d failed requests, want 0 (pages fetched: %v)", r.FailedRequests, hits)
	}
}

func TestAccounting_RobotsBlockedOutsideTotal(t *testing.T) {
	s := &site{
		pages:  map[string][]string{"/": {"/private", "/open"}, "/open": nil, "/private": nil},
		robots: "User-agent: *\nDisallow: /private\n",
	}
	r := crawl(t, s, "/", 10*time.Second, func(c *domain.TesterConfig) { c.IgnoreRobots = false })

	if r.TotalRequests != 2 || r.RobotsBlocked != 1 {
		t.Errorf("total/robots_blocked = %d/%d, want 2/1", r.TotalRequests, r.RobotsBlocked)
	}
	if _, hits := s.pageHits(); hits["/private"] != 0 {
		t.Errorf("/private fetched %d times, want 0", hits["/private"])
	}
	if !r.CrawlComplete {
		t.Errorf("CrawlComplete = false (%s), want true: a blocked URL is not missing work", r.IncompleteReason)
	}
}

// manyPages returns a site whose root links n pages.
func manyPages(n int) *site {
	pages := map[string][]string{"/": nil}
	for i := range n {
		p := fmt.Sprintf("/p%d", i)
		pages["/"] = append(pages["/"], p)
		pages[p] = nil
	}
	return &site{pages: pages}
}

func TestRate_BurstOne(t *testing.T) {
	s := manyPages(40)
	crawl(t, s, "/", 5*time.Second, func(c *domain.TesterConfig) { c.Rate = 5; c.Concurrency = 5 })

	// Burst 1 at 5 req/s over 5s allows 1 + 5*5 = 26 requests (35 with burst 2x rate).
	if total, _ := s.pageHits(); total > 26 {
		t.Errorf("%d page requests in 5s at -rate 5, want at most 26", total)
	}
}

func TestRate_CrawlDelayFloor(t *testing.T) {
	s := manyPages(40)
	s.robots = "User-agent: *\nCrawl-delay: 1\n"
	crawl(t, s, "/", 3*time.Second, func(c *domain.TesterConfig) {
		c.Rate = 5
		c.Concurrency = 5
		c.IgnoreRobots = false
	})

	// Crawl-delay 1 caps the rate at 1 req/s: 1 + 3*1 = 4 requests in 3s.
	if total, _ := s.pageHits(); total > 4 {
		t.Errorf("%d page requests in 3s under Crawl-delay: 1, want at most 4", total)
	}
}

func TestEffectiveRate(t *testing.T) {
	tests := []struct {
		rate  float64
		delay time.Duration
		want  float64
	}{
		{5, 0, 5},
		{5, time.Second, 1},
		{0.5, time.Second, 0.5},
		{10, 500 * time.Millisecond, 2},
	}
	for _, tt := range tests {
		if got := effectiveRate(tt.rate, tt.delay); got != tt.want {
			t.Errorf("effectiveRate(%g, %v) = %g, want %g", tt.rate, tt.delay, got, tt.want)
		}
	}
}

func TestCompleteness_Complete(t *testing.T) {
	r := crawl(t, manyPages(3), "/", 10*time.Second, nil)

	if !r.CrawlComplete || r.IncompleteReason != "" {
		t.Errorf("CrawlComplete = %v, reason %q; want true, none", r.CrawlComplete, r.IncompleteReason)
	}
}

func TestCompleteness_QueueFull(t *testing.T) {
	r := crawl(t, manyPages(5), "/", 10*time.Second, func(c *domain.TesterConfig) {
		c.QueueSize = 1
		c.Concurrency = 1
	})

	if r.CrawlComplete || r.IncompleteReason != domain.IncompleteQueueFull || r.URLsDropped == 0 {
		t.Errorf("complete=%v reason=%q dropped=%d; want false, %q, >0",
			r.CrawlComplete, r.IncompleteReason, r.URLsDropped, domain.IncompleteQueueFull)
	}
}

func TestCompleteness_Duration(t *testing.T) {
	s := manyPages(4)
	s.delay = 300 * time.Millisecond
	r := crawl(t, s, "/", 100*time.Millisecond, nil)

	if r.CrawlComplete || r.IncompleteReason != domain.IncompleteDuration {
		t.Errorf("complete=%v reason=%q; want false, %q", r.CrawlComplete, r.IncompleteReason, domain.IncompleteDuration)
	}
	if r.TotalRequests != 0 || r.FailedRequests != 0 || r.AbandonedRequests == 0 {
		t.Errorf("total/failed/abandoned = %d/%d/%d; a cut-off request must count only as abandoned",
			r.TotalRequests, r.FailedRequests, r.AbandonedRequests)
	}
	if strings.Contains(fmt.Sprint(r.Errors), "canceled") {
		t.Errorf("a cut-off request was recorded as an error: %v", r.Errors)
	}
}
