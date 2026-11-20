---
title: Contributing
nav_order: 8
---

# Contributing to Lobster

Thank you for your interest in contributing to Lobster. This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, inclusive, and professional in all interactions. We're building a welcoming community for everyone.

## How to Contribute

### Reporting Bugs

1. Check existing issues to avoid duplicates
2. Create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (OS, Go version, etc.)
   - Relevant logs or error messages

### Suggesting Features

1. Check existing feature requests
2. Create a new issue with:
   - Clear use case
   - Expected behavior
   - Why this benefits users
   - Potential implementation approach (optional)

### Submitting Code

1. **Fork the repository**
   ```bash
   git clone https://github.com/1mb-dev/lobster.git
   cd lobster
   ```

2. **Create a branch** (`feat/*`, `fix/*` or `docs/*`)
   ```bash
   git checkout -b feat/your-change
   ```

3. **Make your changes**
   - Follow the existing code style
   - Add tests for new functionality
   - Update documentation as needed
   - Keep commits atomic and well-described

4. **Run tests**
   ```bash
   go test ./...
   go vet ./...
   ```

5. **Commit your changes**
   ```bash
   git commit -m "type(scope): description"
   ```

6. **Push to your fork**
   ```bash
   git push origin feat/your-change
   ```

7. **Create a Pull Request**
   - Provide a clear description
   - Link related issues
   - Ensure CI passes

## Development Guidelines

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `go vet` before committing
- Keep functions focused and concise
- Add comments for exported functions

### Testing

- Write unit tests for new functions
- Maintain test coverage >70%
- Test edge cases and error conditions
- Use table-driven tests where appropriate

### Commit Messages

Follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `test`: Test additions/changes
- `chore`: Build/tooling changes

Examples:
```
fix(tester): correct percentile calculation
docs(readme): update installation instructions
```

### Pull Request Guidelines

- Keep PRs focused on a single feature/fix
- Update relevant documentation
- Add tests for new functionality
- Ensure all tests pass
- Address review feedback promptly
- Squash commits before merging (if requested)

## Project Structure

```
lobster/
├── cmd/lobster/          # CLI entry point
├── internal/
│   ├── cli/               # Flag registration, config building, auth, help text
│   ├── config/            # Configuration file loading
│   ├── crawler/           # URL discovery
│   ├── domain/            # Core entities
│   ├── load/              # -load phase and ownership gate
│   ├── reporter/          # Console, JSON and HTML reports
│   ├── robots/            # robots.txt parsing
│   ├── stats/             # Response-time histogram, percentiles
│   ├── tester/            # Crawl: worker pool, rate limit, redirect policy
│   ├── util/              # Address checks, URL validation, sanitization
│   └── validator/         # Performance targets and verdict
├── docs/                  # Documentation site
├── examples/              # Example configurations
└── README.md
```

Flow: crawler → tester → (load) → validator + reporter; `domain` holds the config and result types.

- Keep domain logic independent
- Use interfaces for dependencies
- Avoid circular dependencies

## Getting Help

- [GitHub Discussions](https://github.com/1mb-dev/lobster/discussions)
- [Issue Tracker](https://github.com/1mb-dev/lobster/issues)
- [Documentation](index)

## Recognition

Contributors will be recognized in:
- Release notes for significant contributions
- GitHub contributors page

Thank you for making Lobster better.
