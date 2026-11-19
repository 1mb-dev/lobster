package tester

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/1mb-dev/lobster/v2/internal/robots"
	"github.com/1mb-dev/lobster/v2/internal/util"
)

// maxRedirects is the most redirects one request may follow.
const maxRedirects = 5

// RedirectRefusedError reports a redirect the crawl would not follow.
type RedirectRefusedError struct {
	From, To string
	Reason   string
}

func (e *RedirectRefusedError) Error() string {
	return fmt.Sprintf("redirect from %s to %s refused: %s", e.From, e.To, e.Reason)
}

// redirectPolicy follows a redirect only to the base URL's origin (or its
// http→https upgrade), at most maxRedirects times, and (unless ignoreRobots)
// only to a path robots.txt allows. Staying on the base origin also keeps
// -allow-private-ips scoped to it and keeps custom headers from reaching
// another service or crossing the network unencrypted.
func redirectPolicy(base *url.URL, rp *robots.Parser, ignoreRobots bool) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		refuse := func(reason string) error {
			return &RedirectRefusedError{
				From:   util.SanitizeURLDefault(via[len(via)-1].URL.String()),
				To:     util.SanitizeURLDefault(req.URL.String()),
				Reason: reason,
			}
		}
		if len(via) >= maxRedirects {
			return refuse(fmt.Sprintf("more than %d redirects", maxRedirects))
		}
		if !sameOrigin(base, req.URL) {
			return refuse(fmt.Sprintf("it leaves the base origin %s://%s; to crawl %s://%s, pass it as -url",
				base.Scheme, base.Host, req.URL.Scheme, req.URL.Host))
		}
		if !ignoreRobots && !rp.IsAllowed(req.URL.String()) {
			return refuse("robots.txt disallows the target")
		}
		return nil
	}
}

// sameOrigin reports whether target has base's hostname, scheme and port, or
// is the http→https upgrade of base (port 80 to 443). A downgrade is refused.
func sameOrigin(base, target *url.URL) bool {
	if !strings.EqualFold(base.Hostname(), target.Hostname()) {
		return false
	}
	bs, ts := strings.ToLower(base.Scheme), strings.ToLower(target.Scheme)
	bp, tp := portOf(base), portOf(target)
	if bs == ts {
		return bp == tp
	}
	return bs == "http" && ts == "https" && bp == "80" && tp == "443"
}

// portOf returns u's port, or the scheme's default.
func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

// refusal returns the safety refusal inside err (a blocked dial address or a
// refused redirect), or nil. Use it in place of err when reporting: the
// http.Client wrapper around it prints the unsanitized target URL.
func refusal(err error) error {
	var blocked *util.BlockedAddressError
	if errors.As(err, &blocked) {
		return blocked
	}
	var redirect *RedirectRefusedError
	if errors.As(err, &redirect) {
		return redirect
	}
	return nil
}
