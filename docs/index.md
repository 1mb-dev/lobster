---
title: Home
nav_order: 1
---

# Lobster

Crawl a site you own and check every page it links against performance targets.

Lobster starts at a base URL, follows same-host links, fetches each page it finds, and reports a pass or fail line for each performance target.

## Key Features

- **URL discovery**: follows same-host `<a href>` links up to `-max-depth`
- **Pass/fail per target**: success and error rate, average, p95 and p99 response time, requests per second
- **Rate limiting**: one shared limit across all workers
- **Reports**: console summary; JSON and HTML files with `-output`
- **Private base URLs refused**: localhost and private IPs need `-allow-private-ips`

## Quick Start

### Installation

```bash
go install github.com/1mb-dev/lobster/v2/cmd/lobster@latest
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

See the [Quick Start Guide](quickstart) for more examples.

## Documentation

| Guide | Description |
|-------|-------------|
| [Quick Start](quickstart) | Get up and running in 5 minutes |
| [Configuration](configuration) | CLI flags, environment variables, and config files |

## For Contributors

| Guide | Description |
|-------|-------------|
| [Development](development) | Project structure and development workflow |
| [Testing](testing) | Testing strategy and coverage goals |
| [Contributing](contributing) | How to contribute to Lobster |

## Responsible Use

Only test systems you own or have explicit permission to test. See the [Responsible Use](responsible-use) guidelines.

## Version

Current stable release: **v2.0.2**

See the [Changelog](https://github.com/1mb-dev/lobster/blob/main/CHANGELOG.md) for release history and migration guides.

## Links

- [GitHub Repository](https://github.com/1mb-dev/lobster)
- [Issue Tracker](https://github.com/1mb-dev/lobster/issues)
- [Discussions](https://github.com/1mb-dev/lobster/discussions)
