# Contributing to Obsyk Operator

Thank you for your interest in contributing to the Obsyk Operator! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for everyone.

## Getting Started

### Prerequisites

- Go 1.24+
- Earthly
- Docker (OrbStack recommended for Mac)
- kubectl
- Kind or Minikube (for local testing)
- GitHub CLI (`gh`)

### Development Setup

1. **Fork and clone the repository:**
   ```bash
   gh repo fork obsyk/obsyk-operator --clone
   cd obsyk-operator
   ```

2. **Verify the build works:**
   ```bash
   earthly +ci
   ```

3. **Create a local Kubernetes cluster:**
   ```bash
   kind create cluster --name obsyk-dev
   ```

## Development Workflow

### Branch Naming

Create branches from `main` using the following format:

- Features: `feature/GH-<issue>-<description>`
- Bug fixes: `fix/GH-<issue>-<description>`
- Documentation: `docs/GH-<issue>-<description>`

Example:
```bash
git checkout -b feature/GH-15-rate-limiting
```

### Making Changes

1. **Write code following our conventions** (see below)
2. **Add tests** for new functionality
3. **Run tests locally:**
   ```bash
   # Run all tests with race detection
   go test -race ./... -v

   # Run CI checks
   earthly +ci
   ```
4. **Ensure code is formatted:**
   ```bash
   gofmt -w .
   ```

### Commit Messages

Follow the format:
```
[GH-<issue>] <type>: <description>

<optional body>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `refactor`: Code refactoring (no functional change)
- `test`: Adding or updating tests
- `docs`: Documentation changes
- `chore`: Maintenance tasks

**Examples:**
```
[GH-12] feat: add thread-safety to transport client

Add sync.RWMutex to protect tokenProvider field from concurrent access.
```

### Pull Requests

1. **Push your branch:**
   ```bash
   git push -u origin feature/GH-15-rate-limiting
   ```

2. **Create a PR:**
   ```bash
   gh pr create --title "[GH-15] Add rate limiting" --body "..."
   ```

3. **PR requirements:**
   - All CI checks must pass
   - Tests must cover new functionality
   - Follow the PR template (if provided)
   - Include `Fixes #<issue>` in the description to auto-close the issue

4. **Wait for review** before merging

## Coding Conventions

### Go Code Style

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use table-driven tests
- Wrap errors with context: `fmt.Errorf("getting secret: %w", err)`
- Use structured logging with `logr`

### Thread-Safety

When working with shared state:

1. **Use `sync.RWMutex`** for read-heavy workloads
2. **Double-checked locking** for cache access patterns
3. **Avoid I/O under locks** - fetch data outside the lock
4. **Test with `-race` flag** to detect race conditions

Example:
```go
// Fast path - read lock
r.mu.RLock()
if val, ok := r.cache[key]; ok {
    r.mu.RUnlock()
    return val, nil
}
r.mu.RUnlock()

// Slow path - write lock
r.mu.Lock()
defer r.mu.Unlock()

// Double-check
if val, ok := r.cache[key]; ok {
    return val, nil
}
val := createValue()
r.cache[key] = val
return val, nil
```

### Controller Patterns

- Use controller-runtime's `Watches()` for declarative resource watching
- Use SharedInformerFactory for event-driven streaming
- Never store sensitive data (private keys) in memory longer than necessary
- Never log sensitive data

### Testing

- Unit tests should be in `*_test.go` files alongside the code
- Integration tests use envtest for Kubernetes API mocking
- All tests should pass with `-race` flag
- Aim for >80% code coverage

**Running specific tests:**
```bash
# Single package
go test ./internal/controller/... -v

# Single test
go test ./internal/auth/... -v -run TestTokenManager_GetAccessToken

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Project Structure

```
/api/v1/           - CRD type definitions
/cmd/              - Operator entrypoint
/internal/
  /auth/           - OAuth2 JWT Bearer authentication
  /controller/     - Kubernetes controller logic
  /ingestion/      - Event-driven resource watching
  /transport/      - HTTP client for platform communication
/config/           - Kubernetes manifests (CRD, RBAC, etc.)
/charts/           - Helm chart
```

## Security Considerations

When contributing, please keep in mind:

1. **Never log sensitive data** (private keys, tokens, credentials)
2. **Read secrets at runtime** from Kubernetes Secrets
3. **Use read-only RBAC** - the operator only needs list/watch permissions
4. **Validate all external input** before processing
5. **Use TLS** for all network communication

## Reporting Issues

Use GitHub Issues to report bugs or request features:

1. **Search existing issues** first to avoid duplicates
2. **Use descriptive titles** with issue type prefix
3. **Include reproduction steps** for bugs
4. **Provide context** (Kubernetes version, platform, logs)

## Getting Help

- Check existing documentation in `CLAUDE.md` and `README.md`
- Search closed issues for similar questions
- Open a new issue with the `question` label

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
