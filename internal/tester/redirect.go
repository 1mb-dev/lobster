package tester

import (
	"errors"
	"fmt"
	"net/http"
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

// redirectPolicy follows a redirect only to baseHost, at most maxRedirects
// times, and (unless ignoreRobots) only to a path robots.txt allows. Staying
// on the base host also keeps -allow-private-ips scoped to it and keeps custom
// headers from reaching another host.
func redirectPolicy(baseHost string, rp *robots.Parser, ignoreRobots bool) func(*http.Request, []*http.Request) error {
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
		if !strings.EqualFold(req.URL.Hostname(), baseHost) {
			return refuse(fmt.Sprintf("it leaves the base host %s; to crawl %s, pass it as -url", baseHost, req.URL.Hostname()))
		}
		if !ignoreRobots && !rp.IsAllowed(req.URL.String()) {
			return refuse("robots.txt disallows the target")
		}
		return nil
	}
}

// isRefusal reports whether err is a safety refusal: a blocked dial address or a refused redirect.
func isRefusal(err error) bool {
	var blocked *util.BlockedAddressError
	var redirect *RedirectRefusedError
	return errors.As(err, &blocked) || errors.As(err, &redirect)
}
