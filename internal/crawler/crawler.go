// Package crawler provides URL discovery and link extraction for web crawling.
package crawler

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/util"
)

// Crawler handles URL discovery and link extraction
type Crawler struct {
	discoveredURLs sync.Map
	baseURL        *url.URL
	urlPattern     *regexp.Regexp
	maxDepth       int
	discoveredCnt  atomic.Int64 // O(1) counter for discovered URLs
	droppedCnt     atomic.Int64 // Counter for URLs dropped due to queue full
}

// New creates a new crawler
func New(baseURL string, maxDepth int) (*Crawler, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	return &Crawler{
		baseURL:    parsedURL,
		urlPattern: regexp.MustCompile(`(?i)<a\s+(?:[^>]*?\s)?href\s*=\s*(?:"([^"]*)"|'([^']*)')`),
		maxDepth:   maxDepth,
	}, nil
}

// ExtractLinks extracts the href of every <a> element in an HTML body
func (c *Crawler) ExtractLinks(body string) []string {
	matches := c.urlPattern.FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))

	for _, match := range matches {
		link := strings.TrimSpace(match[1] + match[2]) // one quote style matched
		if c.isValidLink(link) {
			// Decode HTML entities (e.g., &amp; -> &, &quot; -> ")
			decodedLink := html.UnescapeString(link)
			links = append(links, decodedLink)
		}
	}

	return links
}

// isValidLink checks if a link should be followed
func (c *Crawler) isValidLink(link string) bool {
	if link == "" {
		return false
	}

	// Skip javascript:, mailto:, and fragment-only links
	if strings.HasPrefix(link, "javascript:") ||
		strings.HasPrefix(link, "mailto:") ||
		strings.HasPrefix(link, "#") {
		return false
	}

	return true
}

// AddURL adds a URL to the discovery queue if it's valid and not already discovered
func (c *Crawler) AddURL(rawURL string, depth int, urlQueue chan<- domain.URLTask) domain.AddURLResult {
	// Parse and validate URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return domain.AddURLResult{Added: false, Reason: domain.AddURLParseError}
	}

	// Make relative URLs absolute
	if !parsedURL.IsAbs() {
		parsedURL = c.baseURL.ResolveReference(parsedURL)
	}

	// On an https base, an http link on port 80 is fetched over https: a
	// downgrade would send the run's credentials in plaintext.
	if strings.EqualFold(c.baseURL.Scheme, "https") && strings.EqualFold(parsedURL.Scheme, "http") && util.PortOf(parsedURL) == "80" {
		parsedURL.Scheme = "https"
		parsedURL.Host = strings.TrimSuffix(parsedURL.Host, ":80")
	}
	if !util.SameOrigin(c.baseURL, parsedURL) {
		return domain.AddURLResult{Added: false, Reason: domain.AddURLInvalidHost}
	}
	// One spelling per page, for dedup. SameOrigin leaves a scheme change only
	// for an http base on port 80 and an https link on 443.
	if strings.EqualFold(parsedURL.Scheme, c.baseURL.Scheme) {
		parsedURL.Host = c.baseURL.Host
	} else {
		parsedURL.Host = strings.TrimSuffix(c.baseURL.Host, ":80")
	}

	// Clean URL (remove fragment; an empty path is the root, so the base URL
	// and a link to "/" are one URL)
	parsedURL.Fragment = ""
	if parsedURL.Path == "" {
		parsedURL.Path = "/"
	}
	cleanURL := parsedURL.String()

	// Checked before recording the URL, so a page first seen too deep can
	// still be queued if it is found again at an allowed depth.
	if depth > c.maxDepth {
		return domain.AddURLResult{Added: false, Reason: domain.AddURLDepthExceeded}
	}

	// Every queued URL is on the base origin or its https upgrade, so path
	// and query name the page: the http:80 and https:443 spellings are one.
	key := *parsedURL
	key.Scheme, key.User, key.Host = "", nil, ""
	if _, exists := c.discoveredURLs.LoadOrStore(key.String(), true); exists {
		return domain.AddURLResult{Added: false, Reason: domain.AddURLDuplicate}
	}

	// Track discovered URL count (O(1) instead of iterating sync.Map)
	c.discoveredCnt.Add(1)

	// Add to queue
	select {
	case urlQueue <- domain.URLTask{URL: cleanURL, Depth: depth}:
		return domain.AddURLResult{Added: true, Reason: domain.AddURLSuccess}
	default:
		// Queue full - track dropped URLs for visibility
		c.droppedCnt.Add(1)
		return domain.AddURLResult{Added: false, Reason: domain.AddURLQueueFull}
	}
}

// GetDiscoveredCount returns the number of discovered URLs (O(1) operation)
func (c *Crawler) GetDiscoveredCount() int {
	return int(c.discoveredCnt.Load())
}

// GetDroppedCount returns the number of URLs dropped due to queue full
func (c *Crawler) GetDroppedCount() int {
	return int(c.droppedCnt.Load())
}
