---
title: Contributing
nav_order: 6
---

# Contributing

## Issues

A bug report needs the command you ran, what you expected, what happened, and the output of `lobster -version`. Search the [open issues](https://github.com/1mb-dev/lobster/issues) first.

## Pull Requests

You need Go (the version in `go.mod`) and golangci-lint v2.

1. Fork the repository, clone your fork, and branch from `main`: `feat/*`, `fix/*` or `docs/*`.
2. Run `make setup` once to install the tracked git hooks.
3. Add tests with the change. Tests use `httptest` servers, never real endpoints.
4. `make ci` must pass: format, vet, lint, tests under the race detector, build.
5. Commit subjects follow `type(scope): subject`, one concern per commit. Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`.
6. Open the pull request against `main`; CI must pass.

The package layout, the data flow and how to add a flag are in [Development](development).
