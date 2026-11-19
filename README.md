# Lobster

[![CI](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml/badge.svg)](https://github.com/1mb-dev/lobster/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/1mb-dev/lobster/v2.svg)](https://pkg.go.dev/github.com/1mb-dev/lobster/v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Lobster** crawls a site you own from a base URL, fetches each page it finds, and checks response times and success rate against performance targets, with a pass or fail line for each.

## Features

- **URL discovery**: follows same-host `<a href>` links up to `-max-depth`
- **Pass/fail per target**: success and error rate, average, p95 and p99 response time, requests per second
- **Rate limiting**: one token bucket ([goflow](https://github.com/1mb-dev/goflow)) shared by all workers
- **Reports**: console summary; JSON and HTML files with `-output`
- **robots.txt**: respected by default
- **Private base URLs refused**: localhost and private IPs need `-allow-private-ips`
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
go build -o lobster cmd/lobster/main.go
```

### Basic Usage

Check your local application:

```bash
lobster -url http://localhost:3000 -allow-private-ips
```

Check a public site you own:

```bash
lobster -url https://example.com -duration 30s
```

Lobster prints a summary when the run ends. Add `-output results.json` to also write `results.json` and `results.html`.

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

- **What it does:** starting at the base URL, Lobster follows same-host `<a href>` links up to `-max-depth`, fetches each page it finds once through the shared rate limit, and records each status code and response time.
- **What it does not do:** it runs no JavaScript, follows no links to other hosts, and skips pages robots.txt disallows (unless `-ignore-robots`). The run continues until `-duration` ends, even after the last page is fetched, and the exit status does not reflect failed targets.
- **How to read results:** the summary gives totals, success rate and average, minimum and maximum response time; the validation section then gives one PASS or FAIL line per performance target, p95 and p99 included. Success rate counts any HTTP response, 404 included. Requests per second is total requests divided by the whole run duration, so the default 100 req/s target fails on a small site.

## Reports

- **Console:** totals, success rate, response times, a count of URLs per status code, then a PASS or FAIL line per performance target.
- **JSON** (`-output results.json`): the same metrics plus per-URL results, for scripts.
- **HTML:** written beside the JSON file with the same name (`results.html`): response status and response-time charts, per-URL table, errors and slow requests.

## Use Cases

- Checking that every linked page on a site you own responds, and how fast
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
