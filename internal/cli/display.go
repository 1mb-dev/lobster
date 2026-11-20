package cli

import (
	"fmt"
	"io"
	"strings"
)

// PrintWarningBox prints a formatted warning box to w.
// The box has a consistent width and styling for security/ethical warnings.
func PrintWarningBox(w io.Writer, title string, lines []string) {
	const boxWidth = 68 // Inner content width

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "╔%s╗\n", strings.Repeat("═", boxWidth+2))
	fmt.Fprintf(w, "║ %-*s ║\n", boxWidth, CenterText(title, boxWidth))
	fmt.Fprintf(w, "╠%s╣\n", strings.Repeat("═", boxWidth+2))

	for _, line := range lines {
		if line == "" {
			fmt.Fprintf(w, "║ %-*s ║\n", boxWidth, "")
		} else {
			fmt.Fprintf(w, "║  %-*s║\n", boxWidth-1, line)
		}
	}

	fmt.Fprintf(w, "╚%s╝\n", strings.Repeat("═", boxWidth+2))
	fmt.Fprintf(w, "\n")
}

// CenterText centers text within a given width.
func CenterText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}

// ShowHelpMessage prints the help message to w; version is shown as given.
func ShowHelpMessage(w io.Writer, version string) {
	fmt.Fprintln(w, `Lobster - crawl a site you own and check each page against performance targets

USAGE:
    lobster [OPTIONS]

OPTIONS:
    -config string
        Path to configuration file (JSON format)
    -url string
        Base URL to crawl (default: http://localhost:3000)
    -concurrency int
        Number of concurrent workers (default: 5)
    -duration string
        Time limit for the crawl (default: 2m); the run ends sooner when
        every page has been fetched
    -load string
        After a complete crawl, replay the crawled pages that responded
        200-399 at -rate for this long (e.g. 30s): requests go out on
        schedule whatever the response times, up to 256 in flight; a
        request not sent by the end of the window is unsent and fails
        the run, unless no wait for a slot delayed it and a slot is
        free; stops early after 10 consecutive 429, 5xx or connection
        errors.
        A public host, or a robots.txt Crawl-delay, needs -i-own-this and
        LOBSTER_I_OWN_THIS=<host>. -rate is capped at 1000 with -load.
    -i-own-this
        Confirm you own the -load target (with LOBSTER_I_OWN_THIS=<host>)
    -timeout string
        Request timeout (default: 30s)
    -rate float
        Requests per second, shared by all workers (default: 2.0)
        Minimum 0.1; below 1.0 asks for confirmation on a terminal
        robots.txt Crawl-delay lowers the crawl's rate to 1/delay
    -user-agent string
        User agent string (default: lobster/<version>)
    -max-depth int
        Maximum crawl depth; 0 fetches the base page only (default: 3)
    -queue-size int
        URL queue buffer size (default: 10000)
    -insecure-skip-verify
        INSECURE: Skip TLS certificate verification
        Requires LOBSTER_INSECURE_TLS=true
    -allow-private-ips
        Allow a localhost or private-IP base URL
        Only use for services you own or have permission to test
    -ignore-robots
        Ignore robots.txt directives
        Only use if you OWN the website or have explicit permission
    -output string
        JSON results file
    -html string
        HTML report file: summary, status counts, response-time histogram
    -verbose
        Enable verbose logging with structured output
    -no-progress
        Disable real-time progress updates

AUTHENTICATION OPTIONS:
    -auth-type string
        Authentication type: basic, bearer, cookie, header
    -auth-username string
        Username for basic authentication
    -auth-password-stdin
        Read password from stdin (one line)
    -auth-token-stdin
        Read bearer token from stdin (one line)
    -auth-header string
        Custom header name; its value comes from LOBSTER_AUTH_HEADER
        or -auth-header-stdin
    -auth-header-stdin
        Read the -auth-header value from stdin (one line)

    Credentials via environment variables:
        LOBSTER_AUTH_PASSWORD   Password for basic authentication
        LOBSTER_AUTH_TOKEN      Bearer token for authentication
        LOBSTER_AUTH_COOKIE     Cookie string in name=value format
        LOBSTER_AUTH_HEADER     Value for the -auth-header header

    -version
        Show version information
    -help
        Show this help message

EXAMPLES:
    # Crawl a local site
    lobster -url http://localhost:3000 -allow-private-ips

    # Check only the base page
    lobster -url http://localhost:3000 -allow-private-ips -max-depth 0

    # Faster crawl, with JSON and HTML reports
    lobster -url http://localhost:3000 -allow-private-ips -rate 10 \
        -output results.json -html report.html

    # Crawl, then replay the crawled pages at 50 req/s for 30s
    LOBSTER_I_OWN_THIS=localhost lobster -url http://localhost:3000 -allow-private-ips \
        -rate 50 -load 30s -i-own-this

    # Use configuration file
    lobster -config myconfig.json

    # Basic authentication (password from an environment variable)
    LOBSTER_AUTH_PASSWORD=secret lobster -url http://localhost:3000 -allow-private-ips \
        -auth-type basic -auth-username admin

    # Basic authentication (password from stdin)
    echo "secret" | lobster -url http://localhost:3000 -allow-private-ips \
        -auth-type basic -auth-username admin -auth-password-stdin

    # Bearer token (from an environment variable)
    LOBSTER_AUTH_TOKEN=eyJhbGc... lobster -url https://example.com -auth-type bearer

    # Custom header, e.g. an API key (value from an environment variable)
    LOBSTER_AUTH_HEADER=your-key lobster -url https://example.com \
        -auth-type header -auth-header X-API-Key

CONFIGURATION FILE EXAMPLE:
    {
      "base_url": "http://localhost:3000",
      "concurrency": 10,
      "duration": "5m",
      "rate": 10.0,
      "max_depth": 3,
      "output_file": "results.json",
      "auth": {
        "type": "basic",
        "username": "admin",
        "password": "${LOBSTER_AUTH_PASSWORD}"
      },
      "performance_targets": {
        "p95_response_time_ms": 100,
        "success_rate": 99.0
      }
    }

    A target set to 0, or left out, is not judged; success_rate defaults to 99.
    Unknown keys are refused. Use ${VAR_NAME} for environment variable substitution;
    never store plaintext credentials in config files.

DOCUMENTATION:
    Full documentation: https://1mb-dev.github.io/lobster/
    Report issues: https://github.com/1mb-dev/lobster/issues

VERSION:
    Lobster `+version)
}
