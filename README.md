# Lobster

[![CI](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml/badge.svg)](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/1mb-dev/lobster/v2.svg)](https://pkg.go.dev/github.com/1mb-dev/lobster/v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Lobster** crawls a site you own from a base URL, fetches each page it finds, and checks response times and success rate against performance targets. With `-load`, it then replays the pages it found at a fixed rate. The run passes or fails, and the exit code says which.

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

`lobster -help` lists every flag; `-config` takes a JSON file (template: `examples/config.example.json`). Reference: [Configuration Guide](https://1mb-dev.github.io/lobster/configuration).

## How It Works

1. **Crawl.** Starting at the base URL, Lobster follows same-host `<a href>` links up to `-max-depth` and fetches each page once, at most `-rate` requests per second. On an http base that is not redirected to https, a same-host `https://` link is crawled as the https site, so the pages it reaches are fetched over https as well. It skips pages robots.txt disallows and runs no JavaScript. The crawl ends when every page has been fetched; `-duration` is its time limit. A crawl cut short by `-duration` or a full queue, or with a page cut off before its links were read, is incomplete, and the run fails.
2. **Load (optional).** With `-load 30s`, after a complete crawl, Lobster sends one request every `1/-rate` seconds to the crawled pages that responded 200-399, in turn. Requests go out on schedule however slowly the server responds, so a slow server shows in the latency rather than lowering the rate. A public host needs an ownership acknowledgement; see [Configuration](https://1mb-dev.github.io/lobster/configuration#load-phase).
3. **Verdict.** Targets cover success rate, average, p95 and p99 response time, and requests per second under `-load`; each enabled target gets a PASS or FAIL line. Only the success-rate target (99%) is on by default. Exit code 0 means every target passed and the crawl completed; 1 means a target failed, the crawl was incomplete, or the load aborted or left requests unsent; 2 means a usage, configuration or safety refusal, or a runtime error. A CI step can gate on the exit code.

## Reports

Console output from a crawl of a local 21-page site at `-rate 20`:

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

## Responsible Use

Only run Lobster against sites you own or have written permission to test. By default, Lobster:

- respects robots.txt and its `Crawl-delay`
- keeps `-rate` as one limit for all workers
- checks every connection's address: localhost and private addresses need `-allow-private-ips`, and redirects stay on the base URL's origin
- refuses a `-load` against a public host without `-i-own-this` and `LOBSTER_I_OWN_THIS=<host>`
- takes credentials from environment variables or stdin, never from flag values

[Responsible Use](https://1mb-dev.github.io/lobster/responsible-use) covers load guidance and report handling.

## Documentation

- [Quick Start](https://1mb-dev.github.io/lobster/quickstart)
- [Configuration Reference](https://1mb-dev.github.io/lobster/configuration)
- [Development Guide](https://1mb-dev.github.io/lobster/development) and [Contributing](https://1mb-dev.github.io/lobster/contributing)
- [Changelog](CHANGELOG.md)
- [Issues](https://github.com/1mb-dev/lobster/issues)

## License

MIT, see [LICENSE](LICENSE).
