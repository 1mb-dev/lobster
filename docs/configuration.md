---
title: Configuration
nav_order: 3
---

# Configuration Reference

Lobster is configured with CLI flags, a JSON configuration file, and environment variables for credentials. `lobster -help` lists every flag.

## How a configuration is built

1. **Defaults**, listed in the tables below.
2. **Configuration file** (`-config`): each key in the file replaces its default; a key left out keeps it. An unknown key is an error.
3. **CLI flags**: a flag replaces the file's value only when it is set on the command line.
4. **Validation**: an invalid value stops the run with exit code 2 before any request is sent.

```bash
# The file sets concurrency 5; the flag overrides it with 20
lobster -config config.json -concurrency 20
```

## CLI Flags

### Crawl

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-url` | string | `http://localhost:3000` | Base URL to crawl |
| `-concurrency` | int | 5 | Number of concurrent workers |
| `-duration` | duration | 2m | Time limit for the crawl; the run ends sooner when every page has been fetched |
| `-timeout` | duration | 30s | HTTP request timeout |
| `-rate` | float | 2.0 | Requests per second, shared by all workers (minimum 0.1) |
| `-max-depth` | int | 3 | Maximum crawl depth; 0 fetches the base page only |
| `-queue-size` | int | 10000 | URL queue buffer size |
| `-user-agent` | string | `lobster/<version>` | User-Agent header |
| `-ignore-robots` | bool | false | Ignore robots.txt directives |

### Load

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-load` | duration | none | After a complete crawl, replay the crawled URLs at `-rate` for this long |
| `-i-own-this` | bool | false | Confirm you own the `-load` target; also needs `LOBSTER_I_OWN_THIS=<host>` |

### Safety

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-allow-private-ips` | bool | false | Allow a localhost or private-IP target |
| `-insecure-skip-verify` | bool | false | Skip TLS certificate verification; also needs `LOBSTER_INSECURE_TLS=true` |

### Authentication

| Flag | Type | Description |
|------|------|-------------|
| `-auth-type` | string | `basic`, `bearer`, `cookie` or `header` |
| `-auth-username` | string | Username for basic authentication |
| `-auth-password-stdin` | bool | Read the password from stdin (one line) |
| `-auth-token-stdin` | bool | Read the bearer token from stdin (one line) |
| `-auth-header` | string | Custom header name; the value comes from `LOBSTER_AUTH_HEADER` or `-auth-header-stdin` |
| `-auth-header-stdin` | bool | Read the `-auth-header` value from stdin (one line) |

Only one of the three `-*-stdin` flags can be used per run.

### Output

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-output` | string | none | JSON results file |
| `-html` | string | none | HTML report file |
| `-verbose` | bool | false | Verbose structured logging |
| `-no-progress` | bool | false | Disable the progress line |

### Other

| Flag | Description |
|------|-------------|
| `-config` | Path to a JSON configuration file |
| `-version` | Show the version |
| `-help` | Show the help message |

## Environment Variables

Secrets are read from the environment or stdin, never from flag values, which show in process lists and shell history.

| Variable | Description |
|----------|-------------|
| `LOBSTER_AUTH_PASSWORD` | Password for basic authentication |
| `LOBSTER_AUTH_TOKEN` | Bearer token |
| `LOBSTER_AUTH_COOKIE` | Cookie, as `name=value` |
| `LOBSTER_AUTH_HEADER` | Value for the `-auth-header` header |
| `LOBSTER_INSECURE_TLS` | Must be `true` for `-insecure-skip-verify` |
| `LOBSTER_I_OWN_THIS` | Must equal the target's host name for a `-load` that needs acknowledgement |

```bash
# Password from the environment
export LOBSTER_AUTH_PASSWORD="secret"
lobster -url http://localhost:3000 -allow-private-ips -auth-type basic -auth-username admin

# API key header, value from stdin
vault kv get -field=key secret/api | \
  lobster -url https://example.com -auth-type header -auth-header X-API-Key -auth-header-stdin
```

## Configuration File

```json
{
  "base_url": "http://localhost:3000",
  "concurrency": 10,
  "duration": "5m",
  "timeout": "30s",
  "rate": 10.0,
  "max_depth": 3,
  "queue_size": 10000,
  "load": "",
  "user_agent": "",
  "ignore_robots": false,
  "insecure_skip_verify": false,
  "verbose": false,
  "output_file": "results.json",
  "html_file": "report.html",
  "auth": {
    "type": "basic",
    "username": "admin",
    "password": "${LOBSTER_AUTH_PASSWORD}"
  },
  "performance_targets": {
    "requests_per_second": 0,
    "avg_response_time_ms": 0,
    "p95_response_time_ms": 100,
    "p99_response_time_ms": 0,
    "success_rate": 99.0
  }
}
```

An empty `load` means no load phase; an empty `user_agent` means `lobster/<version>`.

### Environment Variable Substitution

`${VAR_NAME}` in the file is replaced with the variable's value; `${VAR_NAME:-default}` falls back to a default. An unset variable without a default is an error.

```json
{
  "auth": {
    "type": "bearer",
    "token": "${API_TOKEN}"
  }
}
```

### Auth Configuration Options

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `basic`, `bearer`, `cookie` or `header` |
| `username` | string | Username for basic auth |
| `password` | string | Password (use `${ENV_VAR}`) |
| `token` | string | Bearer token (use `${ENV_VAR}`) |
| `cookies` | object | Name-value pairs for cookie auth |
| `headers` | object | Name-value pairs for header auth |
| `cookie_file` | string | Path to a Netscape-format cookie file |

### Performance Targets

A target of 0 is off. Only `success_rate` is on by default.

| Field | Default | Passes when |
|-------|---------|-------------|
| `success_rate` | 99.0 | the percentage of requests with a 200-399 status is at least this |
| `avg_response_time_ms` | 0 (off) | the average response time is at most this |
| `p95_response_time_ms` | 0 (off) | the 95th-percentile response time is at most this |
| `p99_response_time_ms` | 0 (off) | the 99th-percentile response time is at most this |
| `requests_per_second` | 0 (off) | the `-load` phase's achieved rate is at least this; skipped without `-load` |

Percentiles use the nearest-rank method over a histogram whose buckets are about 9% wide, so a reported percentile is at most one bucket above the exact value.

With `-load`, the latency and success-rate targets judge the load phase, and the crawl is judged by its own success rate.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | The crawl completed and every enabled target passed |
| 1 | A target failed, the crawl was incomplete, or the load phase aborted or left requests unsent |
| 2 | Usage or configuration error, a refused base URL, a refused `-load`, or a runtime error |

The JSON report's `performance_validation.overall_status` is `PASS` or `FAIL`, matching exit codes 0 and 1.

```bash
lobster -url http://localhost:3000 -allow-private-ips -config targets.json -no-progress
echo "exit code: $?"
```

A crawl is complete when every discovered URL was requested: none dropped from a full queue (raise `-queue-size`) and none cut off by `-duration` (raise `-duration` or `-rate`). The JSON report gives `crawl_complete` and, when it is false, `incomplete_reason`.

## Load Phase

`-load 30s` runs after a complete crawl. It sends one request every `1/-rate` seconds to the crawled URLs in turn:

- Requests go out on schedule however slowly the server responds, and latency is measured from each request's scheduled send time, so a slow server shows in the latency instead of lowering the rate.
- At most 256 requests are in flight; a request sent after waiting for a free slot is counted as late.
- No request due after the `-load` window is sent. A request still waiting for a free slot when the window ends is counted as unsent and fails the run; the run ends once the requests in flight finish (within `-timeout`).
- 429 responses are not retried. After 10 consecutive 429 or 5xx responses the load stops, and the run exits 1.
- `-rate` is capped at 1000 with `-load`. `-concurrency` applies to the crawl only.

A load against a host that resolves to a public address, or whose robots.txt sets a `Crawl-delay`, needs both `-i-own-this` and `LOBSTER_I_OWN_THIS` set to the host name. `-allow-private-ips` does not count. A load that needs neither can connect only to private addresses.

```bash
LOBSTER_I_OWN_THIS=staging.example.com lobster -url https://staging.example.com \
  -rate 50 -load 1m -i-own-this -output results.json
```

## Examples

```bash
# Local application with defaults
lobster -url http://localhost:3000 -allow-private-ips

# Base page only
lobster -url http://localhost:3000 -allow-private-ips -max-depth 0

# Faster crawl with a longer time limit, JSON and HTML reports
lobster -url http://localhost:3000 -allow-private-ips -rate 10 -duration 5m \
  -output results.json -html report.html

# Internal service with a self-signed certificate
export LOBSTER_INSECURE_TLS=true
lobster -url https://dev.internal:8443 -insecure-skip-verify -allow-private-ips
```

## Tuning

### Rate and Concurrency

`-rate` is one limit shared by all workers, with a burst of 1: requests are spaced `1/-rate` seconds apart whatever `-concurrency` is. A robots.txt `Crawl-delay` lowers the crawl's rate to one request per delay. Below 1.0 req/s, Lobster asks for confirmation on a terminal.

More workers help when responses are slow: with a 1s response time and `-rate 10`, about 10 workers keep the rate.

### Queue Size

The default of 10,000 suits most sites. If the run reports `incomplete_reason: queue_full`, raise `-queue-size`.

### robots.txt

Lobster respects robots.txt by default, and checks it again on every redirect. Use `-ignore-robots` only when you own the site or have explicit permission.
