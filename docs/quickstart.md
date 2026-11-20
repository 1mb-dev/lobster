---
title: Quick Start
nav_order: 2
---

# Quick Start Guide

Get up and running with Lobster in 5 minutes.

## Installation

### From Source

```bash
git clone https://github.com/1mb-dev/lobster.git
cd lobster
go build -o lobster ./cmd/lobster
```

### Using Go Install

```bash
go install github.com/1mb-dev/lobster/v2/cmd/lobster@latest
```

## Basic Usage

### 1. Check Your Local Application

Start your application on port 3000, then run:

```bash
lobster -url http://localhost:3000 -allow-private-ips
```

This will:
- Follow same-host links from the base page up to depth 3 and fetch each page once (on an http base, pages reached through an `https://` link are fetched over https as well)
- Space requests at 2 per second (default `-rate`), shared by 5 workers
- Stop when every page has been fetched, or after 2 minutes (default `-duration`)
- Print a summary and a PASS or FAIL line per performance target, and exit 0 on PASS, 1 on FAIL

`-allow-private-ips` is required for localhost and private addresses.

### 2. Customize the Crawl

```bash
lobster \
  -url http://localhost:3000 \
  -allow-private-ips \
  -rate 10 \
  -duration 5m \
  -output results.json \
  -html report.html
```

Parameters:
- `-rate 10`: up to 10 requests per second in total
- `-duration 5m`: give the crawl up to 5 minutes
- `-output results.json`: write the results as JSON
- `-html report.html`: write an HTML report of the crawl with a response-time histogram

### 3. Configure Performance Targets

Create a configuration file `config.json`:

```json
{
  "base_url": "http://localhost:3000",
  "rate": 10.0,
  "output_file": "results.json",
  "performance_targets": {
    "p95_response_time_ms": 100,
    "success_rate": 99.0
  }
}
```

Run with config:
```bash
lobster -config config.json -allow-private-ips
```

A target left out, or set to 0, is not judged. Only `success_rate` (99%) is on by default.

### 4. Add a Load Phase

After a complete crawl, `-load` replays the pages it found at `-rate` for a set time:

```bash
lobster -url http://localhost:3000 -allow-private-ips -rate 50 -load 30s
```

Against a public host, or one whose robots.txt sets a `Crawl-delay`, Lobster also needs `-i-own-this` and `LOBSTER_I_OWN_THIS=<host>`. See [Configuration](configuration#load-phase).

## Understanding Results

The console shows the crawl's totals, success rate and response times (average, minimum, maximum, p95, p99), whether the crawl completed, a count of URLs per status code, the load phase if one ran, and one PASS or FAIL line per enabled target.

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

**Key Metrics:**
- **Success Rate**: the percentage of requests that got a 200-399 status
- **p95 / p99**: 95% (99%) of requests were at least this fast, to within about 9%
- **Crawl Complete**: every discovered page was requested; an incomplete crawl fails the run
- **Requests/Second**: requests divided by the crawl's duration; with `-load`, the load phase reports its achieved rate

## With Authentication

For authenticated endpoints, use environment variables:

```bash
# Basic authentication
export LOBSTER_AUTH_PASSWORD="secret"
lobster -url http://localhost:3000 -allow-private-ips -auth-type basic -auth-username admin

# Bearer token
export LOBSTER_AUTH_TOKEN="your-api-token"
lobster -url http://localhost:3000 -allow-private-ips -auth-type bearer
```

See [Configuration](configuration) for all authentication options.

## Next Steps

- [Configuration Reference](configuration) - All CLI flags and options
- [Responsible Use](responsible-use) - Only test systems you own
- [Contributing](contributing) - How to contribute

## Getting Help

- [GitHub Discussions](https://github.com/1mb-dev/lobster/discussions)
- [Report Issues](https://github.com/1mb-dev/lobster/issues)
