---
title: Testing
nav_order: 7
---

# Testing Strategy

This document describes the testing approach for Lobster, including test categories, coverage goals, and how to run tests locally.

## Test Categories

### Unit Tests
Fast, isolated tests of single functions, such as `internal/stats` and `internal/util`.

### Integration Tests
Runs against `httptest` servers: `internal/tester/integration_test.go`, `internal/tester/accounting_test.go`, `internal/load/load_test.go`.

### Contract Tests
`cmd/lobster/main_test.go` runs the whole command through `run()` and checks exit codes, output and the JSON keys against `cmd/lobster/testdata/*.golden`.

### Timing Tests
Some tests measure rates and deadlines on real clocks (the rate, Crawl-delay and load tests) and take a few seconds each.

## Coverage

```bash
make test-coverage        # per-function coverage and the total
make test-coverage-html   # browsable report
```

Aim for 70% or more overall, and cover every error path a user can reach (refused URLs, cancelled requests, 429 backoff).

### What to Test

- Business logic and algorithms
- Error handling paths (HTTP 429 retry, context cancellation, refused connections and redirects)
- Edge cases and boundary conditions
- Public APIs and exported functions
- Critical user workflows (crawl → load → verdict → report)

### What Not to Test

- Third-party library internals
- Standard library behavior
- Trivial functions

## Running Tests

```bash
make test                                    # all packages, race detector, integration tests included
go test ./internal/tester -v                 # one package
go test ./internal/tester -run TestRedirect  # matching tests
go test ./cmd/lobster -run TestRun_JSONKeysMatchGolden -update   # regenerate the JSON key golden files
```

`make test` runs every test, integration tests included; only two slow tester tests skip under `go test -short`. Tests use `httptest` servers only, never real endpoints.

## Continuous Integration

CI runs the shared 1mb-dev Go workflow: lint, `go test -race ./...` on two Go versions, gitleaks, govulncheck and a build.
