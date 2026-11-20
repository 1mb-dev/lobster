package crawler

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/1mb-dev/lobster/v2/internal/domain"
)

// page is the URL the bodies in these tests were served from.
var page = &url.URL{Scheme: "http", Host: "example.com", Path: "/"}

func extract(t *testing.T, c *Crawler, body string, page *url.URL) []string {
	t.Helper()
	links, err := c.ExtractLinks(strings.NewReader(body), page)
	if err != nil {
		t.Fatal(err)
	}
	return links
}

func TestNew_Success(t *testing.T) {
	c, err := New("http://example.com", 3)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if c == nil {
		t.Fatal("Expected non-nil crawler")
	}
	if c.maxDepth != 3 {
		t.Errorf("Expected maxDepth 3, got %d", c.maxDepth)
	}
}

func TestNew_InvalidURL(t *testing.T) {
	_, err := New(":", 3)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestExtractLinks_BasicHTML(t *testing.T) {
	c, _ := New("http://example.com", 3)

	html := `<html>
		<body>
			<a href="/page1">Page 1</a>
			<a href="/page2">Page 2</a>
			<a href="http://example.com/page3">Page 3</a>
		</body>
	</html>`

	links := extract(t, c, html, page)

	if len(links) != 3 {
		t.Errorf("Expected 3 links, got %d", len(links))
	}

	expectedLinks := map[string]bool{
		"http://example.com/page1": true,
		"http://example.com/page2": true,
		"http://example.com/page3": true,
	}

	for _, link := range links {
		if !expectedLinks[link] {
			t.Errorf("Unexpected link: %s", link)
		}
	}
}

func TestExtractLinks_DoubleQuotes(t *testing.T) {
	c, _ := New("http://example.com", 3)

	html := `<a href="http://example.com/double">Link</a>`
	links := extract(t, c, html, page)

	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0] != "http://example.com/double" {
		t.Errorf("Expected 'http://example.com/double', got '%s'", links[0])
	}
}

func TestExtractLinks_SingleQuotes(t *testing.T) {
	c, _ := New("http://example.com", 3)

	html := `<a href='http://example.com/single'>Link</a>`
	links := extract(t, c, html, page)

	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0] != "http://example.com/single" {
		t.Errorf("Expected 'http://example.com/single', got '%s'", links[0])
	}
}

func TestExtractLinks_HTMLEntities(t *testing.T) {
	c, _ := New("http://example.com", 3)

	html := `<a href="/path?param1=value&amp;param2=value">Link</a>`
	links := extract(t, c, html, page)

	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}

	// Should decode &amp; to &
	expected := "http://example.com/path?param1=value&param2=value"
	if links[0] != expected {
		t.Errorf("Expected '%s', got '%s'", expected, links[0])
	}
}

func TestExtractLinks_InvalidLinks(t *testing.T) {
	c, _ := New("http://example.com", 3)

	html := `<html>
		<body>
			<a href="javascript:void(0)">JS Link</a>
			<a href="mailto:test@example.com">Email</a>
			<a href="#">Fragment only</a>
			<a href="">Empty</a>
			<a href="   ">Whitespace</a>
		</body>
	</html>`

	links := extract(t, c, html, page)

	if len(links) != 0 {
		t.Errorf("Expected 0 valid links, got %d: %v", len(links), links)
	}
}

// Only <a> elements are followed, whatever their other attributes, quotes or
// case; nothing in a comment, script or style is a link.
func TestExtractLinks_AnchorsOnly(t *testing.T) {
	c, _ := New("http://example.com", 3)

	body := `<link rel="icon" href="/favicon.ico">
		<link rel="stylesheet" href="/s.css">
		<area shape="rect" href="/area"><abbr title="x">y</abbr>
		<a href="/a">a</a> <a class="x" href='/b'>b</a> <A HREF="/c">c</A>
		<a
		  data-href="/no" href = "/d">d</a> <a href="/it's">e</a>
		<a title="a > b" href="/gt">f</a> <a onclick="if(a>b)go()" href="/js">g</a>
		<a title="see href='/y'" href="/x">h</a> <a href=/pricing>i</a>
		<a href="/q?a=1&amp;b=2">j</a> <a href="/self"/>
		<!-- <a href="/comment">k</a> -->
		<script>var s = '<a href="/script">';</script>
		<style>/* <a href="/style"> */</style>`
	got := strings.Join(extract(t, c, body, page), " ")
	want := "http://example.com/a http://example.com/b http://example.com/c http://example.com/d http://example.com/it's " +
		"http://example.com/gt http://example.com/js http://example.com/x http://example.com/pricing " +
		"http://example.com/q?a=1&b=2 http://example.com/self"
	if got != want {
		t.Errorf("links = %q,\nwant %q", got, want)
	}
}

// Links resolve against the first <base href>, itself resolved against the
// page, wherever the links appear; without one, against the page.
func TestExtractLinks_BaseHref(t *testing.T) {
	c, _ := New("http://example.com", 3)
	docs := &url.URL{Scheme: "http", Host: "example.com", Path: "/docs/x"}

	tests := []struct{ name, body, want string }{
		{"no base", `<a href="about">`, "http://example.com/docs/about"},
		{"root base", `<base href="/"><a href="about">`, "http://example.com/about"},
		{"base after the link", `<a href="about"><base href="/">`, "http://example.com/about"},
		{"relative base", `<base href="v2/"><a href="about">`, "http://example.com/docs/v2/about"},
		{"first base wins", `<base href="/one/"><base href="/two/"><a href="about">`, "http://example.com/one/about"},
		{"base without href", `<base target="_top"><base href="/"><a href="about">`, "http://example.com/about"},
		{"absolute path ignores base", `<base href="/one/"><a href="/about">`, "http://example.com/about"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(extract(t, c, tt.body, docs), " "); got != tt.want {
				t.Errorf("links = %q, want %q", got, tt.want)
			}
		})
	}
}

// lobster runs no scripts, so links inside <noscript> are followed, wherever
// the noscript sits and whether or not it is closed.
func TestExtractLinks_Noscript(t *testing.T) {
	c, _ := New("http://example.com", 3)

	tests := []struct{ name, body, want string }{
		{"in body", `<noscript><a href="/nojs">x</a></noscript><a href="/after">y</a>`, "http://example.com/nojs http://example.com/after"},
		{"in head", `<head><noscript><a href="/h"></noscript></head><body><a href="/after">`, "http://example.com/h http://example.com/after"},
		{"unclosed", `<noscript><a href="/u"><p><a href="/after">`, "http://example.com/u http://example.com/after"},
		{"script inside", `<noscript><script>'<a href="/s">'</script><a href="/n"></noscript>`, "http://example.com/n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(extract(t, c, tt.body, page), " "); got != tt.want {
				t.Errorf("links = %q, want %q", got, tt.want)
			}
		})
	}
}

// An href is read as a browser reads it: tabs and line breaks anywhere are
// dropped, and only http and https links, in any case, are kept.
func TestExtractLinks_HrefAsBrowserReads(t *testing.T) {
	c, _ := New("http://example.com", 3)

	body := "<a href=\"/docs/\ngetting-started\">a</a> <a href=\"/t\tab\">b</a> <a href=\"/c\r\nr\">c</a>" +
		`<a href="JavaScript:void(0)">d</a> <a href="MAILTO:x@example.com">e</a> <a href="tel:+15550100">f</a>` +
		"<a href=\"java\nscript:alert(1)\">g</a> <a href=\"data:text/html,x\">h</a> <a href=\"HTTP://example.com/up\">i</a>"
	got := strings.Join(extract(t, c, body, page), " ")
	if want := "http://example.com/docs/getting-started http://example.com/tab http://example.com/cr http://example.com/up"; got != want {
		t.Errorf("links = %q,\nwant %q", got, want)
	}
}

// A read error cuts extraction short and is returned.
func TestExtractLinks_ReadError(t *testing.T) {
	c, _ := New("http://example.com", 3)
	reset := errors.New("connection reset")
	body := io.MultiReader(strings.NewReader(`<a href="/x">`), iotest.ErrReader(reset))
	if _, err := c.ExtractLinks(body, page); !errors.Is(err, reset) {
		t.Errorf("err = %v, want %v", err, reset)
	}
}

func TestIsValidLink(t *testing.T) {
	c, _ := New("http://example.com", 3)

	tests := []struct {
		name  string
		link  string
		valid bool
	}{
		{"valid http", "http://example.com/page", true},
		{"valid relative", "/page", true},
		{"valid relative with query", "/page?query=value", true},
		{"javascript", "javascript:void(0)", false},
		{"mailto", "mailto:test@example.com", false},
		{"fragment only", "#section", false},
		{"javascript, mixed case", "JavaScript:void(0)", false},
		{"mailto, upper case", "MAILTO:test@example.com", false},
		{"tel", "tel:+15550100", false},
		{"http, upper case", "HTTP://example.com/page", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.isValidLink(tt.link)
			if result != tt.valid {
				t.Errorf("isValidLink(%s) = %v, want %v", tt.link, result, tt.valid)
			}
		})
	}
}

func TestAddURL_SameDomain(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	result := c.AddURL("http://example.com/page1", 1, urlQueue)
	if !result.Added {
		t.Errorf("Expected URL to be added, got reason: %s", result.Reason)
	}
	if result.Reason != domain.AddURLSuccess {
		t.Errorf("Expected reason %s, got %s", domain.AddURLSuccess, result.Reason)
	}

	if len(urlQueue) != 1 {
		t.Errorf("Expected 1 URL in queue, got %d", len(urlQueue))
	}

	task := <-urlQueue
	if task.URL != "http://example.com/page1" {
		t.Errorf("Expected URL 'http://example.com/page1', got '%s'", task.URL)
	}
	if task.Depth != 1 {
		t.Errorf("Expected Depth 1, got %d", task.Depth)
	}
}

func TestAddURL_DifferentDomain(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	result := c.AddURL("http://different.com/page", 1, urlQueue)
	if result.Added {
		t.Error("Expected URL from different domain to be rejected")
	}
	if result.Reason != domain.AddURLInvalidHost {
		t.Errorf("Expected reason %s, got %s", domain.AddURLInvalidHost, result.Reason)
	}

	if len(urlQueue) != 0 {
		t.Errorf("Expected 0 URLs in queue, got %d", len(urlQueue))
	}
}

func TestAddURL_RelativeURL(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	result := c.AddURL("/page2", 1, urlQueue)
	if !result.Added {
		t.Errorf("Expected relative URL to be added, got reason: %s", result.Reason)
	}

	task := <-urlQueue
	// Should be converted to absolute
	if task.URL != "http://example.com/page2" {
		t.Errorf("Expected absolute URL 'http://example.com/page2', got '%s'", task.URL)
	}
}

func TestAddURL_Deduplication(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	// Add same URL twice
	result1 := c.AddURL("http://example.com/page", 1, urlQueue)
	result2 := c.AddURL("http://example.com/page", 1, urlQueue)

	if !result1.Added {
		t.Errorf("Expected first URL to be added, got reason: %s", result1.Reason)
	}
	if result2.Added {
		t.Error("Expected duplicate URL to be rejected")
	}
	if result2.Reason != domain.AddURLDuplicate {
		t.Errorf("Expected reason %s, got %s", domain.AddURLDuplicate, result2.Reason)
	}

	if len(urlQueue) != 1 {
		t.Errorf("Expected 1 URL in queue (deduplicated), got %d", len(urlQueue))
	}
}

func TestAddURL_FragmentRemoval(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	// Add URLs with fragments - they should be treated as the same URL
	result1 := c.AddURL("http://example.com/page#section1", 1, urlQueue)
	result2 := c.AddURL("http://example.com/page#section2", 1, urlQueue)

	if !result1.Added {
		t.Errorf("Expected first URL to be added, got reason: %s", result1.Reason)
	}
	if result2.Added {
		t.Error("Expected URL with different fragment to be deduplicated")
	}
	if result2.Reason != domain.AddURLDuplicate {
		t.Errorf("Expected reason %s, got %s", domain.AddURLDuplicate, result2.Reason)
	}

	task := <-urlQueue
	// Fragment should be removed
	if task.URL != "http://example.com/page" {
		t.Errorf("Expected URL without fragment 'http://example.com/page', got '%s'", task.URL)
	}
}

func TestAddURL_MaxDepthExceeded(t *testing.T) {
	c, _ := New("http://example.com", 2) // max depth is 2
	urlQueue := make(chan domain.URLTask, 10)

	// Try to add URL at depth 3 (exceeds maxDepth)
	result := c.AddURL("http://example.com/page", 3, urlQueue)
	if result.Added {
		t.Error("Expected URL exceeding max depth to be rejected")
	}
	if result.Reason != domain.AddURLDepthExceeded {
		t.Errorf("Expected reason %s, got %s", domain.AddURLDepthExceeded, result.Reason)
	}

	if len(urlQueue) != 0 {
		t.Errorf("Expected 0 URLs in queue, got %d", len(urlQueue))
	}
}

func TestGetDiscoveredCount(t *testing.T) {
	c, _ := New("http://example.com", 3)
	urlQueue := make(chan domain.URLTask, 10)

	initialCount := c.GetDiscoveredCount()
	if initialCount != 0 {
		t.Errorf("Expected initial count 0, got %d", initialCount)
	}

	c.AddURL("http://example.com/page1", 1, urlQueue)
	c.AddURL("http://example.com/page2", 1, urlQueue)
	c.AddURL("http://example.com/page1", 1, urlQueue) // duplicate, shouldn't count

	count := c.GetDiscoveredCount()
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestGetDroppedCount(t *testing.T) {
	c, _ := New("http://example.com", 3)
	// Create a queue with capacity of 2
	urlQueue := make(chan domain.URLTask, 2)

	// Initial dropped count should be 0
	if dropped := c.GetDroppedCount(); dropped != 0 {
		t.Errorf("Expected initial dropped count 0, got %d", dropped)
	}

	// Fill the queue
	c.AddURL("http://example.com/page1", 1, urlQueue)
	c.AddURL("http://example.com/page2", 1, urlQueue)

	// Queue is full - this should be dropped
	c.AddURL("http://example.com/page3", 1, urlQueue)
	c.AddURL("http://example.com/page4", 1, urlQueue)

	// Should have 2 dropped URLs
	droppedCount := c.GetDroppedCount()
	if droppedCount != 2 {
		t.Errorf("Expected dropped count 2, got %d", droppedCount)
	}

	// Discovered count should still include all unique URLs
	// (they were seen even if not queued)
	discoveredCount := c.GetDiscoveredCount()
	if discoveredCount != 4 {
		t.Errorf("Expected discovered count 4, got %d", discoveredCount)
	}
}

// A link is queued on the base origin or its http to https upgrade; on an
// https base, an http link on port 80 is queued as https. want is the queued
// URL, empty when the link is refused.
func TestAddURL_SchemeDowngrade(t *testing.T) {
	tests := []struct {
		name, base, final, link, want string
	}{
		{"https base, http link", "https://example.com", "", "http://example.com/x", "https://example.com/x"},
		{"https base, http link on port 80", "https://example.com", "", "http://example.com:80/x", "https://example.com/x"},
		{"https base, http link on another port", "https://example.com", "", "http://example.com:8080/x", ""},
		{"https base on another port, http link", "https://example.com:8443", "", "http://example.com/x", ""},
		{"http base served over https, https link", "http://example.com", "https://example.com/", "https://example.com/x", "https://example.com/x"},
		{"http base served over https, https link on port 443", "http://example.com", "https://example.com/", "https://example.com:443/x", "https://example.com/x"},
		{"http base on port 80 served over https, https link", "http://example.com:80", "https://example.com/", "https://EXAMPLE.com:443/x", "https://example.com/x"},
		{"http base on an IPv6 address served over https, https link", "http://[::1]:80", "https://[::1]/", "https://[::1]:443/x", "https://[::1]/x"},
		{"http base served over http, https link", "http://example.com", "http://example.com/", "https://example.com:443/x", "http://example.com/x"},
		{"same scheme", "https://example.com", "", "https://example.com/x", "https://example.com/x"},
		{"same scheme, default port spelled out", "https://example.com", "", "https://EXAMPLE.com:443/x", "https://example.com/x"},
		{"upgrade on a custom port", "http://example.com:8080", "", "https://example.com:8080/x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := New(tt.base, 3)
			if tt.final != "" {
				final, _ := url.Parse(tt.final)
				c.SetBaseFinalURL(final)
			}
			queue := make(chan domain.URLTask, 1)
			res := c.AddURL(tt.link, 1, queue)
			got := ""
			if res.Added {
				got = (<-queue).URL
			}
			if got != tt.want {
				t.Errorf("AddURL(%s) on %s queued %q, want %q", tt.link, tt.base, got, tt.want)
			}
		})
	}
}

// The http:80 and https:443 spellings of a page are one URL: an http base
// that redirects to https is not crawled twice. The first spelling is queued.
func TestAddURL_SchemePairIsOnePage(t *testing.T) {
	tests := []struct{ base, first, second string }{
		{"http://example.com", "http://example.com/", "https://example.com/"},
		{"http://example.com", "https://example.com/a", "http://example.com/a"},
		{"http://example.com:80", "http://example.com:80/a?q=1", "https://example.com:443/a?q=1"},
		{"https://example.com", "https://example.com/a", "http://example.com/a"},
	}
	for _, tt := range tests {
		c, _ := New(tt.base, 3)
		queue := make(chan domain.URLTask, 2)
		if res := c.AddURL(tt.first, 1, queue); !res.Added {
			t.Fatalf("%s on %s: %s", tt.first, tt.base, res.Reason)
		}
		if res := c.AddURL(tt.second, 1, queue); res.Reason != domain.AddURLDuplicate {
			t.Errorf("%s after %s on %s: %s, want %s", tt.second, tt.first, tt.base, res.Reason, domain.AddURLDuplicate)
		}
	}
}

// The http:80 and https:443 spellings of a page are queued with the scheme the
// base page was served over, whichever spelling a link uses and whichever is
// found first.
func TestAddURL_SpellingFollowsBaseNotOrder(t *testing.T) {
	tests := []struct{ name, final, want string }{
		{"base stayed on http", "http://example.com/", "http://example.com/p"},
		{"base redirected to https", "https://example.com/home", "https://example.com/p"},
	}
	for _, tt := range tests {
		for _, order := range [][]string{
			{"http://example.com/p", "https://example.com/p"},
			{"https://example.com/p", "http://example.com/p"},
		} {
			c, _ := New("http://example.com", 3)
			final, _ := url.Parse(tt.final)
			c.SetBaseFinalURL(final)
			queue := make(chan domain.URLTask, 2)
			for _, link := range order {
				c.AddURL(link, 1, queue)
			}
			if len(queue) != 1 {
				t.Fatalf("%s, %v: %d queued, want 1", tt.name, order, len(queue))
			}
			if got := (<-queue).URL; got != tt.want {
				t.Errorf("%s, %v: queued %s, want %s", tt.name, order, got, tt.want)
			}
		}
	}
}
