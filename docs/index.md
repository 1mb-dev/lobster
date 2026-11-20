---
title: Home
nav_order: 1
---

# Lobster

Crawl a site you own and check every page it links against performance targets.

Lobster starts at a base URL, follows same-host links, fetches each page it finds, and reports a pass or fail line for each performance target. With `-load`, it then replays the pages it found at a fixed rate. The exit code says whether the run passed.

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
lobster -url https://example.com
```

See the [Quick Start Guide](quickstart) for more examples.

## Responsible Use

Only test systems you own or have explicit permission to test. See the [Responsible Use](responsible-use) guidelines.

Release history and upgrade notes: [Changelog](https://github.com/1mb-dev/lobster/blob/main/CHANGELOG.md). Bugs: [issues](https://github.com/1mb-dev/lobster/issues).
