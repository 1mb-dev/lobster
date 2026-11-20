// Package crawler provides URL discovery and link extraction for web crawling.
package crawler

import (
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/util"
)

// Crawler handles URL discovery and link extraction
type Crawler struct {
	discoveredURLs sync.Map
	baseURL        *url.URL
	scheme         atomic.Pointer[string] // the scheme pages on the base origin are queued with
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

	c := &Crawler{
		baseURL:  parsedURL,
		maxDepth: maxDepth,
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	c.scheme.Store(&scheme)
	return c, nil
}

// SetBaseFinalURL records the URL the base page was served from, after
// redirects: an http base on port 80 that ended on https queues its pages
// over https, and one that stayed on http queues them over http, whichever
// spelling a link uses. A URL off the base origin is ignored.
func (c *Crawler) SetBaseFinalURL(final *url.URL) {
	if util.SameOrigin(c.baseURL, final) {
		scheme := strings.ToLower(final.Scheme)
		c.scheme.Store(&scheme)
	}
}

// ExtractLinks returns the href of every <a> element in an HTML body, resolved
// against the document's base URL: the first <base href>, itself resolved
// against page, or page when there is none. page is the URL the body was
// served from, after redirects. Only http and https links are returned.
func (c *Crawler) ExtractLinks(body string, page *url.URL) []string {
	base, baseSet := page, false
	var hrefs []string
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, _ := z.TagName()
		switch string(name) {
		case "noscript":
			// lobster runs no scripts, so noscript content is markup
			z.NextIsNotRawText()
		case "a":
			if href, ok := hrefOf(z); ok {
				hrefs = append(hrefs, href)
			}
		case "base":
			if href, ok := hrefOf(z); ok && !baseSet {
				baseSet = true
				if u, err := page.Parse(href); err == nil {
					base = u
				}
			}
		}
	}

	return c.resolve(hrefs, base)
}

// resolve returns the hrefs that lead to another page, as http or https URLs
// resolved against base.
func (c *Crawler) resolve(hrefs []string, base *url.URL) []string {
	links := make([]string, 0, len(hrefs))
	for _, href := range hrefs {
		if !c.isValidLink(href) {
			continue
		}
		if u, err := base.Parse(href); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			links = append(links, u.String())
		}
	}
	return links
}

// hrefOf returns the current tag's first href attribute, entity-decoded, with
// tabs and line breaks removed and spaces trimmed, as a browser reads it.
func hrefOf(z *html.Tokenizer) (string, bool) {
	for {
		key, val, more := z.TagAttr()
		if string(key) == "href" {
			return strings.TrimSpace(stripTabsAndNewlines.Replace(string(val))), true
		}
		if !more {
			return "", false
		}
	}
}

var stripTabsAndNewlines = strings.NewReplacer("\t", "", "\n", "", "\r", "")

// isValidLink reports whether an href can lead to another page: not empty,
// not fragment-only, and with no scheme or an http or https one.
func (c *Crawler) isValidLink(link string) bool {
	if link == "" || strings.HasPrefix(link, "#") {
		return false
	}
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https")
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
	// One spelling per page: SameOrigin leaves a scheme change only for an
	// http base on port 80 and an https link on 443, and both are queued with
	// the scheme the base page was served over.
	parsedURL.Scheme = *c.scheme.Load()
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
