# Changelog

All notable changes to the Obsyk Operator are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Thread-safety for transport client and controller ([GH-12])
  - `sync.RWMutex` protection for `agentClients` map in controller
  - `sync.RWMutex` protection for `tokenProvider` in transport client
  - Double-checked locking pattern for optimal performance
  - Concurrent access tests with race detector validation

### Changed
- Updated CLAUDE.md with comprehensive architecture documentation
- Added CONTRIBUTING.md with contribution guidelines
- Added CHANGELOG.md for version tracking

## [0.1.0] - 2024-11-28

### Added
- Event-driven delta streaming with SharedInformerFactory ([GH-11])
  - `IngestionManager` for coordinating resource watching
  - `PodIngester`, `ServiceIngester`, `NamespaceIngester` implementations
  - Buffered event channel with configurable size (default: 1000)
  - Non-blocking event sends with graceful degradation
  - ResourceVersion-based deduplication for update events
  - Graceful shutdown with event draining
  - Comprehensive unit and integration tests

- OAuth2 JWT Bearer authentication ([GH-17])
  - `TokenManager` for JWT assertion creation and token refresh
  - ECDSA P-256 key pair support
  - Automatic token caching and refresh before expiry
  - Thread-safe credential updates

- HTTP transport client ([GH-5])
  - `SendSnapshot()`, `SendEvent()`, `SendHeartbeat()` methods
  - Retry with exponential backoff for 5xx errors
  - Custom error types (`AuthError`, `ServerError`, `ClientError`)

- ObsykAgent CRD and controller ([GH-3], [GH-4])
  - Custom Resource Definition with spec and status
  - Reconciliation loop with resource watches
  - Cluster UID auto-detection from kube-system namespace
  - Status conditions (Available, Syncing, Degraded)
  - Resource count tracking

- Helm chart for installation ([GH-6])
  - Configurable values for agent and operator
  - RBAC resources (ClusterRole, ClusterRoleBinding)
  - Security context with non-root user
  - Leader election support

- E2E test workflow ([GH-16], [GH-18])
  - Kind cluster creation and teardown
  - Operator deployment verification
  - Sync status validation

- CI/CD pipelines
  - PR checks with Earthly (lint, test, build)
  - Release workflow with GHCR publishing
  - Trivy security scanning
  - Helm chart publishing to GitHub Pages

### Fixed
- Helm chart versioning automation ([GH-28])
- Helm chart publishing and repository setup ([GH-26])
- Security-events permission for Trivy SARIF upload ([GH-25])
- JSON field alignment with platform API
- Domain migration from obsyk.com to obsyk.ai ([GH-23])

### Security
- Read-only RBAC (list, watch only for Pods, Services, Namespaces)
- Credentials stored in Kubernetes Secrets only
- Private keys never logged or exposed
- TLS for all platform communication

## Initial Setup

### Added
- Go project structure with Earthly build ([GH-1])
- Repository documentation and CI/CD workflows
- Apache License 2.0

---

[Unreleased]: https://github.com/Obsyk/obsyk-operator/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Obsyk/obsyk-operator/releases/tag/v0.1.0

[GH-1]: https://github.com/Obsyk/obsyk-operator/issues/1
[GH-3]: https://github.com/Obsyk/obsyk-operator/issues/3
[GH-4]: https://github.com/Obsyk/obsyk-operator/issues/4
[GH-5]: https://github.com/Obsyk/obsyk-operator/issues/5
[GH-6]: https://github.com/Obsyk/obsyk-operator/issues/6
[GH-11]: https://github.com/Obsyk/obsyk-operator/issues/11
[GH-12]: https://github.com/Obsyk/obsyk-operator/issues/12
[GH-16]: https://github.com/Obsyk/obsyk-operator/issues/16
[GH-17]: https://github.com/Obsyk/obsyk-operator/issues/17
[GH-18]: https://github.com/Obsyk/obsyk-operator/issues/18
[GH-23]: https://github.com/Obsyk/obsyk-operator/issues/23
[GH-25]: https://github.com/Obsyk/obsyk-operator/issues/25
[GH-26]: https://github.com/Obsyk/obsyk-operator/issues/26
[GH-28]: https://github.com/Obsyk/obsyk-operator/issues/28
