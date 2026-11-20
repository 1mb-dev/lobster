---
title: Development
nav_order: 6
---

# Development Guide

## Project Overview

**Lobster** crawls a site from a base URL, fetches each page it finds, and checks response times and success rate against performance targets. With `-load`, it then replays the crawled pages open-loop at a fixed rate.

**Repository**: https://github.com/1mb-dev/lobster
**License**: MIT

## What Makes Lobster Different

1. **Crawler-first**: discovers the URLs to check (ab, wrk, hey and vegeta take a URL list)
2. **Pass/fail**: targets and an exit code, not only metrics
3. **Polite by default**: one rate limit shared by all workers, robots.txt and its `Crawl-delay` respected
4. **Guarded connections**: every dialed address is checked, redirects stay on the base origin, and a load against a public host needs an ownership acknowledgement

## Architecture

```
cmd/lobster/            # run(): flags, exit codes, crawl then optional load
internal/cli/           # Flag registration, config building, auth, help text
internal/config/        # Configuration file loading (strict keys, ${VAR} substitution)
internal/domain/        # Config, results and their validation
internal/crawler/       # Link extraction, same-host filter, deduplication
internal/tester/        # Crawl: worker pool, rate limit, redirect policy, completeness
internal/load/          # -load: open-loop schedule, ownership gate
internal/stats/         # Fixed log-bucket histogram, nearest-rank percentiles
internal/validator/     # Targets and the PASS/FAIL verdict
internal/reporter/      # Console, JSON and HTML reports
internal/robots/        # robots.txt parsing
internal/util/          # Address checks, URL validation, sanitization
```

Flow: `crawler` → `tester` → (`load`) → `validator` + `reporter`.

## Development Workflow

### Setup

```bash
git clone https://github.com/1mb-dev/lobster.git
cd lobster
make build
./lobster -url http://localhost:3000 -allow-private-ips
```

`make help` lists all build targets.

### Running Tests

```bash
make test           # every package under the race detector, integration tests included
make test-verbose   # the same, verbose
make test-coverage  # coverage report
make lint           # golangci-lint
go test ./internal/tester -run TestRedirect                      # matching tests in one package
go test ./cmd/lobster -run TestRun_JSONKeysMatchGolden -update   # regenerate the JSON key golden files
```

- Tests use `httptest` servers only, never real endpoints; set `IgnoreRobots: true` unless robots.txt is under test.
- `cmd/lobster/main_test.go` runs the whole command through `run()` and checks exit codes, output and the JSON keys against `cmd/lobster/testdata/*.golden`. A JSON key change fails it until the golden files are regenerated.
- The rate, Crawl-delay and load tests run on real clocks and take seconds each.

## Key Implementation Details

### Crawl

- One goflow token bucket with a burst of 1 spaces requests `1/-rate` apart across all workers; a robots.txt `Crawl-delay` lowers the rate to `1/delay`.
- A pending counter tracks queued and in-flight URLs; the run ends when it reaches zero, or at the `-duration` deadline. The URL queue is never closed.
- The crawl is complete when nothing is pending, no URL was dropped from a full queue, and no request was cut off by the deadline, and no page's body was cut off before its links were read (the links read before the cut are still queued).
- Links come from streaming up to `MaxResponseSize` (10MB default) of each HTML page through `html.Tokenizer`; a larger `Content-Length` skips the page. When the base page is served over https (after redirects), http links on port 80 are fetched over https and the two spellings of a page are one; no link is fetched over a weaker scheme than it names.

### Load

- Request `i` is due at `start + i/rate`; the schedule never waits for responses, and latency is measured from the due time.
- Up to 256 requests are in flight; a request that waits for a slot, and every request after it due before that slot came free, is counted late. No request due after the window is sent, and none waits for a slot after it; a request not sent by its end goes out only if it fell due after the latest waited-for slot came free and a slot is free, and is otherwise unsent and fails the run.
- Ten consecutive 429 or 5xx responses abort the load.

### Connection Safety

- `util.DialControl` checks every address at dial time, after DNS resolution; there is no proxy.
- The redirect policy follows only the base origin (or its http-to-https upgrade), at most 5 hops, and checks robots.txt again.
- A load that needed no ownership acknowledgement also refuses public addresses at dial time.

### Metrics

Response times go into a fixed histogram of 224 log buckets (8 per octave), so memory does not grow with the number of requests. Percentiles use the nearest rank and report the upper bound of the bucket holding it: at most about 9% above the exact value.

## Key Dependencies

**goflow** (v1.6.0): token bucket rate limiting for the crawl (`internal/tester`).

**golang.org/x/net** (v0.62.0): `html.Tokenizer` for link extraction (`internal/crawler`).

## Common Tasks

### Adding a Flag

1. Register it in `internal/cli/config.go` (`RegisterConfigFlags` and `flagSetters`), or in `cmd/lobster/main.go` if it is not part of the configuration.
2. Add the field and its JSON key to `domain.Config`, and validate it in `Config.Validate`.
3. Add it to the help text in `internal/cli/display.go` and to `docs/configuration.md`.

### Release Checklist

1. Update `CHANGELOG.md`.
2. `make ci` passes.
3. Merge to `main`; wait for CI to pass on the merge commit.
4. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z"` and push the tag.

`-version` reads the version from the build: `make build` passes `git describe` through ldflags, and `go install` records the module version.

## Git Workflow

### Commit Message Format

```
type(scope): description
```

**Types**: feat, fix, docs, refactor, test, chore, perf

**Examples**:
```
feat(load): add the ownership gate
fix(crawler): normalize an empty path to /
docs(readme): update installation instructions
```

### Branch Strategy

- `main`: stable, releasable code
- `feat/*`, `fix/*`, `docs/*`: work branches

## Troubleshooting

### The crawl is incomplete
**Symptom**: `Crawl Complete: no (duration)`, `(queue_full)` or `(links_cut_off)`
**Fix**: raise `-duration` or `-rate` for `duration`; raise `-queue-size` for `queue_full`; raise `-timeout` or `-duration` for `links_cut_off`, and check the page named in the errors

### Many 429 or timeout errors
**Fix**: lower `-rate`, raise `-timeout`

### Only the base page is fetched
**Fix**: check that the page has `<a href>` links to the same host, and that `-max-depth` is above 0

## Resources

- [goflow](https://github.com/1mb-dev/goflow)
- [Go testing](https://go.dev/doc/tutorial/add-a-test)
- [Semantic versioning](https://semver.org/)

## Contact

- **Issues**: https://github.com/1mb-dev/lobster/issues
- **Discussions**: https://github.com/1mb-dev/lobster/discussions
- **Pull Requests**: see [contributing](contributing)
