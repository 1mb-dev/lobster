# Changelog

All notable changes to Lobster will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.1.0] - 2026-11-20

### Added

- `-load <duration>`: after a complete crawl, replays the crawled pages that responded 200-399 open-loop at `-rate` for that long, up to 256 in flight, with latency measured from each request's scheduled send time; stops early after 10 consecutive 429/5xx responses; a request not sent by the end of the window is unsent and fails the run, unless no wait for a slot delayed it and a slot is free
- `-load` ownership gate: a public host, or a robots.txt `Crawl-delay`, needs `-i-own-this` and `LOBSTER_I_OWN_THIS=<host>`
- `-html <path>` writes the HTML report, with no external scripts
- `LOBSTER_AUTH_HEADER` and `-auth-header-stdin` supply the `-auth-header` value
- Exit codes: 0 pass; 1 a failed target, an incomplete crawl, or a load that aborted or left requests unsent; 2 a usage, configuration or safety refusal, or a runtime error
- JSON keys `crawl_complete`, `incomplete_reason`, `abandoned_requests`, `robots_blocked`, `urls_dropped`, `p95_response_time`, `p99_response_time`, `response_time_histogram` and `load`
- robots.txt `Crawl-delay` lowers the crawl rate to one request per delay

### Changed

- A run ends when the crawl completes; `-duration` is the crawl's time limit
- Only a 200-399 status counts as success
- The verdict under `overall_status` is PASS or FAIL; an incomplete crawl fails
- Only the success-rate target (99%) is on by default; a target of 0 is off; the requests-per-second target is judged only with `-load`
- Targets compare inclusively (success rate ≥ target, latency ≤ target); percentiles use the nearest-rank method
- Response times are counted in a fixed log-bucket histogram; reported percentiles are within one bucket (about 9%) of the exact value
- `-rate` is one limit shared by all workers, with a burst of 1
- `-rate` below 0.1, `-rate` above 1000 with `-load`, `-concurrency 0` and zero durations are refused instead of adjusted
- `-max-depth 0` fetches the base page only
- Config files refuse unknown keys; a key absent from the file keeps its default; a flag overrides the file only when set
- `-auth-header` takes the header name only
- The default user agent is `lobster/<version>`; `-version` reports the module version
- URLs robots.txt disallows, and URLs beyond `-max-depth`, are no longer counted as requests; requests cut off by `-duration` count as abandoned
- `-output` writes the JSON file only

### Removed

- `-dry-run`, `-follow-links`, `-respect-429` (429 backoff is always on) and `-compare`
- Config keys `follow_links`, `respect_429`, `dry_run` and `error_rate`
- The error-rate target and the three-tier verdicts
- The per-request `response_times` list in the JSON report

### Security

- Every connection's address is checked when it is dialed, so a hostname that resolves or rebinds to a private address is refused; IPv4-mapped, NAT64, 6to4, Teredo, 198.18.0.0/15 and `.localhost` addresses are covered
- Redirects stay on the base URL's origin (or its http-to-https upgrade), at most 5, and robots.txt is checked again; a refused redirect of the base URL exits 2 and names the target
- Crawled links follow the same origin rule as redirects; on an https base, a same-host http link on port 80 is fetched over https, and fails the run if it fails there
- robots.txt is fetched through the crawl's connection checks, and no proxy is used
- A `-load` that needed no ownership acknowledgement can connect only to private addresses
- The `-auth-header` value no longer appears on the command line

## [2.0.2] - 2026-02-15

### Fixed

- Module path is now `github.com/1mb-dev/lobster/v2`, so `go install github.com/1mb-dev/lobster/v2/cmd/lobster@latest` resolves this major version

## [2.0.1] - 2026-02-15

### Changed

- Repository moved to the 1mb-dev organization; module path `github.com/1mb-dev/lobster`
- Rate limiting uses `github.com/1mb-dev/goflow` v1.5.1, which drops the Prometheus dependencies

## [2.0.0] - 2026-01-15

Major security hardening release with significant performance improvements.

### Added

- **Environment variable substitution in config files**: Use `${VAR_NAME}` or `${VAR_NAME:-default}` syntax to reference environment variables in JSON config files
- **Secure credential input**: `--auth-password-stdin` and `--auth-token-stdin` flags for piping secrets safely
- **Environment variable auth support**: `LOBSTER_AUTH_PASSWORD`, `LOBSTER_AUTH_TOKEN`, `LOBSTER_AUTH_COOKIE` environment variables
- **URL validation with SSRF protection**: Blocks `file://`, `ftp://`, and private IP ranges by default
- **`--allow-private-ips` flag**: Explicitly enable testing against internal/localhost URLs when needed
- **Response size limit**: link extraction skips HTML responses whose Content-Length exceeds 10MB
- **Dropped URL tracking**: Warns when URLs are dropped due to queue overflow with actionable hints
- **Config and auth validation methods**: Early detection of invalid configuration before test execution

### Changed

- **Credential handling (breaking)**: Passwords, tokens, and cookies can no longer be passed via CLI flags. Use environment variables or stdin instead
- **Insecure TLS confirmation (breaking)**: `--insecure-skip-verify` now requires `LOBSTER_INSECURE_TLS=true` environment variable confirmation
- **HTTP connection pooling**: Significantly improved throughput with proper `http.Transport` configuration
- **GetDiscoveredCount performance**: Changed from O(n) to O(1) using atomic counters
- **Result channel buffering**: Dynamic buffer sizing based on concurrency to prevent blocking under load
- **HTTP/2 enabled by default**: `ForceAttemptHTTP2: true` for HTTPS connections
- **429 retry timing**: Response time now correctly reflects only the actual request duration, not retry wait time
- **CLI architecture**: Extracted CLI functions into `internal/cli` package for improved testability and maintainability

### Removed

- **`--auth-password` CLI flag**: Use `LOBSTER_AUTH_PASSWORD` env var or `--auth-password-stdin` instead
- **`--auth-token` CLI flag**: Use `LOBSTER_AUTH_TOKEN` env var or `--auth-token-stdin` instead
- **`--auth-cookie` CLI flag**: Use `LOBSTER_AUTH_COOKIE` env var instead

**Note:** `--auth-header` flag remains available as header names are not secrets.

### Fixed

- Error message sanitization to prevent leaking internal infrastructure details (IPs, hostnames)
- Robots.txt wildcard pattern matching now correctly handles `*` patterns per RFC specification
- Username no longer logged in debug output (prevents enumeration)
- Cookie and header validation now provides explicit error messages for malformed input
- Mutual exclusivity check for `--auth-password-stdin` and `--auth-token-stdin`
- Environment variable substitution properly distinguishes empty values from unset variables
- EOF handling uses `errors.Is(err, io.EOF)` instead of string comparison

### Security

- **Credentials no longer exposed in process list**: Passwords and tokens removed from CLI flags
- **SSRF protection**: Private IP ranges (127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16) blocked by default
- **URL scheme validation**: Only `http://` and `https://` schemes permitted
- **Insecure TLS requires explicit consent**: Two-factor confirmation (flag + env var) for skipping TLS verification
- **Response size limits**: Prevents memory exhaustion from malicious large responses
- **Error message sanitization**: Internal infrastructure details redacted from user-facing output

### Migration Guide

#### Credential Handling

Before (v1.0.0):
```bash
lobster -url https://api.example.com -auth-type basic -auth-username admin -auth-password secret
```

After (v2.0.0):
```bash
# Option 1: Environment variable
LOBSTER_AUTH_PASSWORD=secret lobster -url https://api.example.com -auth-type basic -auth-username admin

# Option 2: Stdin (for scripts)
echo "secret" | lobster -url https://api.example.com -auth-type basic -auth-username admin -auth-password-stdin
```

#### Config File Credentials

Before (v1.0.0):
```json
{
  "auth": {
    "type": "bearer",
    "token": "my-secret-token"
  }
}
```

After (v2.0.0):
```json
{
  "auth": {
    "type": "bearer",
    "token": "${API_TOKEN}"
  }
}
```

Then run with:
```bash
API_TOKEN=my-secret-token lobster -config config.json
```

#### Insecure TLS

Before (v1.0.0):
```bash
lobster -url https://self-signed.example.com -insecure-skip-verify
```

After (v2.0.0):
```bash
LOBSTER_INSECURE_TLS=true lobster -url https://self-signed.example.com -insecure-skip-verify
```

#### Testing Internal Services

Before (v1.0.0):
```bash
lobster -url http://localhost:8080
```

After (v2.0.0):
```bash
lobster -url http://localhost:8080 -allow-private-ips
```

---

## [1.0.0] - 2025-10-26

Initial production release of Lobster, a lightweight HTTP load testing and link validation tool.

### Added

- HTTP stress testing with configurable concurrency and rate limiting
- Recursive link discovery and validation
- Support for basic, bearer, cookie, and header-based authentication
- robots.txt compliance (with `-ignore-robots` override)
- HTTP 429 rate limit handling with exponential backoff
- Real-time progress bar with request statistics
- HTML and JSON report generation
- Dry-run mode for URL discovery without load testing
- Configurable request timeout and test duration
- Performance targets with P95/P99 latency thresholds

[Unreleased]: https://github.com/1mb-dev/lobster/compare/v2.1.0...HEAD
[2.1.0]: https://github.com/1mb-dev/lobster/compare/v2.0.2...v2.1.0
[2.0.2]: https://github.com/1mb-dev/lobster/compare/v2.0.1...v2.0.2
[2.0.1]: https://github.com/1mb-dev/lobster/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/1mb-dev/lobster/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/1mb-dev/lobster/releases/tag/v1.0.0
