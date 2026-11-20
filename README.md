# Lobster

[![CI](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml/badge.svg)](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/1mb-dev/lobster/v2.svg)](https://pkg.go.dev/github.com/1mb-dev/lobster/v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Lobster** crawls a site you own from a base URL, fetches each page it finds, and checks response times and success rate against performance targets. With `-load`, it then replays the pages it found at a fixed rate. The run passes or fails, and the exit code says which.

## Features

- **URL discovery**: follows same-host `<a href>` links up to `-max-depth`
- **Pass/fail per target**: success rate, average, p95 and p99 response time, and requests per second under `-load`; exit code 0 on pass, 1 on fail
- **Load phase**: `-load` replays the crawled pages open-loop at `-rate`, measuring latency from each request's scheduled send time
- **Rate limiting**: one token bucket ([goflow](https://github.com/1mb-dev/goflow)) shared by all workers; robots.txt `Crawl-delay` lowers it
- **Reports**: console summary; JSON with `-output`, HTML with `-html`
- **robots.txt**: respected by default
- **Private addresses refused**: every connection is checked, redirects stay on the base host; localhost and private IPs need `-allow-private-ips`
- **Credentials**: from environment variables or stdin, never flag values

## Quick Start

### Installation

```bash
go install github.com/1mb-dev/lobster/v2/cmd/lobster@latest
```

Or clone and build:

```bash
git clone https://github.com/1mb-dev/lobster.git
cd lobster
go build -o lobster ./cmd/lobster
```

### Basic Usage

Check your local application:

```bash
lobster -url http://localhost:3000 -allow-private-ips
```

Check a public site you own:

```bash
lobster -url https://example.com
```

Lobster prints a summary when the run ends. Add `-output results.json` for JSON, `-html report.html` for an HTML report.

Crawl, then replay the pages that responded 200-399 at 50 req/s for 30 seconds:

```bash
lobster -url http://localhost:3000 -allow-private-ips -rate 50 -load 30s
```

### With Authentication

```bash
# Basic authentication
export LOBSTER_AUTH_PASSWORD="secret"
lobster -url https://api.example.com -auth-type basic -auth-username admin

# Bearer token
export LOBSTER_AUTH_TOKEN="your-api-token"
lobster -url https://api.example.com -auth-type bearer
```

For more examples, see the [Quick Start Guide](https://1mb-dev.github.io/lobster/quickstart).

## Configuration

Key flags: `-url`, `-concurrency`, `-duration`, `-rate`, `-max-depth`, `-output`

Use `-config config.json` for complex setups. See `examples/config.example.json` for template.

Full reference: [Configuration Guide](https://1mb-dev.github.io/lobster/configuration)

## How It Works

1. **Crawl.** Starting at the base URL, Lobster follows same-host `<a href>` links up to `-max-depth` and fetches each page once, at most `-rate` requests per second. On an http base that is not redirected to https, a same-host `https://` link is crawled as the https site, so the pages it reaches are fetched over https as well. It skips pages robots.txt disallows and runs no JavaScript. The crawl ends when every page has been fetched; `-duration` is its time limit. A crawl cut short by `-duration` or a full queue, or with a page cut off before its links were read, is incomplete, and the run fails.
2. **Load (optional).** With `-load 30s`, after a complete crawl, Lobster sends one request every `1/-rate` seconds to the crawled pages that responded 200-399, in turn. Requests go out on schedule however slowly the server responds, so a slow server shows in the latency rather than lowering the rate. A public host needs an ownership acknowledgement; see [Configuration](https://1mb-dev.github.io/lobster/configuration#load-phase).
3. **Verdict.** Each enabled target gets a PASS or FAIL line. Only the success-rate target (99%) is on by default. Exit code 0 means every target passed and the crawl completed; 1 means a target failed, the crawl was incomplete, or the load aborted or left requests unsent; 2 means a usage, configuration or safety refusal.

## Reports

Console output from a crawl of a local 21-page site:

```
============================================================
RESULTS
============================================================
Duration:             1.001655626s
URLs Discovered:      21
Total Requests:       21
Successful Requests:  21
Failed Requests:      0
Average Response Time: 863.559µs
Min Response Time:    257.549µs
Max Response Time:    5.663142ms
p95 / p99:            1.448154ms / 5.663142ms
Requests/Second:      20.97
Success Rate:         100.00%
Crawl Complete:       yes

------------------------------------------------------------
URL VALIDATION SUMMARY
------------------------------------------------------------
HTTP 200: 21 URL(s)
============================================================


============================================================
PERFORMANCE TARGET VALIDATION
============================================================
PASS Success Rate:                  100.00%
    Target: ≥ 99.0%


Overall: PASS (1/1 targets met)
```

- **JSON** (`-output results.json`): the same metrics, per-URL results, a response-time histogram, the load phase if one ran, and `performance_validation.overall_status` (`PASS` or `FAIL`).
- **HTML** (`-html report.html`): summary cards, status counts, per-URL table, errors, slow requests and a response-time histogram for the crawl (no load phase or verdict), with no external scripts.

## Use Cases

- Checking that every linked page on a site you own responds, and how fast
- Failing a deployment pipeline step when a page breaks or slows down (exit code 1)
- Replaying a site's pages at a fixed rate against a staging host you own
- Comparing response times before and after a change

## Documentation

- [Quick Start](https://1mb-dev.github.io/lobster/quickstart)
- [Configuration Reference](https://1mb-dev.github.io/lobster/configuration)
- [Development Guide](https://1mb-dev.github.io/lobster/development)
- [Changelog](https://github.com/1mb-dev/lobster/blob/main/CHANGELOG.md)

## Contributing

Contributions are welcome. See [Contributing Guidelines](https://1mb-dev.github.io/lobster/contributing).

### Development

```bash
git clone https://github.com/1mb-dev/lobster.git
cd lobster
make help      # See all targets
make test      # Run tests
make build     # Build binary
```

## Responsible Use

**Only test systems you own or have written permission to test.**

Unauthorized testing may violate computer fraud laws. Respect robots.txt (enabled by default), use rate limits, handle reports securely.

See [Responsible Use](https://1mb-dev.github.io/lobster/responsible-use) for full guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- [Documentation](https://1mb-dev.github.io/lobster/)
- [Issues](https://github.com/1mb-dev/lobster/issues)
- [Discussions](https://github.com/1mb-dev/lobster/discussions)

Built with [goflow](https://github.com/1mb-dev/goflow)
