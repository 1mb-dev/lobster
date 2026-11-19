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
go build -o lobster cmd/lobster/main.go
```

### Using Go Install

```bash
go install github.com/1mb-dev/lobster/v2/cmd/lobster@latest
```

## Basic Usage

### 1. Test Your Local Application

Start your application on port 3000, then run:

```bash
lobster -url http://localhost:3000 -allow-private-ips
```

This will:
- Run for 2 minutes (default duration), even if the last page is fetched sooner
- Use 5 concurrent workers (default concurrency) sharing one rate limit
- Discover same-host linked pages up to depth 3 and fetch each once
- Print a summary and a pass or fail line per performance target

Note: The `-allow-private-ips` flag is required for localhost and private network testing (SSRF protection).

### 2. Customize the Test

```bash
lobster \
  -url http://localhost:3000 \
  -allow-private-ips \
  -duration 5m \
  -concurrency 10 \
  -rate 5.0 \
  -output results.json
```

Parameters:
- `-duration 5m`: Run for 5 minutes
- `-concurrency 10`: Use 10 concurrent workers
- `-rate 5.0`: Limit to 5 requests per second in total
- `-output results.json`: Save results to `results.json` and `results.html`

### 3. View Results

After running with `-output`, you'll get:
- `results.json`: Machine-readable results
- `results.html`: Interactive HTML report

Open the HTML report in your browser:
```bash
open results.html  # macOS
xdg-open results.html  # Linux
```

### 4. Configure Performance Targets

Create a configuration file `config.json`:

```json
{
  "base_url": "http://localhost:3000",
  "concurrency": 10,
  "duration": "5m",
  "rate": 10.0,
  "output_file": "results.json",
  "performance_targets": {
    "requests_per_second": 100,
    "avg_response_time_ms": 50,
    "p95_response_time_ms": 100,
    "p99_response_time_ms": 200,
    "success_rate": 99.0,
    "error_rate": 1.0
  }
}
```

Run with config:
```bash
lobster -config config.json -allow-private-ips
```

Lobster validates performance against configured targets and reports pass/fail status.

## Understanding Results

Console output has a summary (totals, success rate, average, minimum and maximum response time, a count of URLs per status code), then one PASS or FAIL line per performance target.

**Key Metrics:**
- **Success Rate**: share of requests that got an HTTP response, whatever its status (a 404 counts as a success; check the per-status counts)
- **p95 Response Time**: 95% of requests were faster than this value
- **Requests/Second**: total requests divided by the whole run duration; on a small site it stays low, so the default 100 req/s target fails

The exit status does not reflect failed targets.

## With Authentication

For authenticated endpoints, use environment variables:

```bash
# Basic authentication
export LOBSTER_AUTH_PASSWORD="secret"
lobster -url https://api.example.com -auth-type basic -auth-username admin

# Bearer token
export LOBSTER_AUTH_TOKEN="your-api-token"
lobster -url https://api.example.com -auth-type bearer
```

See [Configuration](configuration) for all authentication options.

## Next Steps

- [Configuration Reference](configuration) - All CLI flags and options
- [Responsible Use](responsible-use) - Only test systems you own
- [Contributing](contributing) - How to contribute

## Getting Help

- [GitHub Discussions](https://github.com/1mb-dev/lobster/discussions)
- [Report Issues](https://github.com/1mb-dev/lobster/issues)
