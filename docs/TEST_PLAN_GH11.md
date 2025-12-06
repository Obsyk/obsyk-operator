# Test Plan: Event-Driven Delta Streaming (GH-11)

This document describes the test plan for verifying the event-driven delta streaming implementation using SharedInformerFactory.

## Implementation Summary

The ingestion package (`internal/ingestion/`) implements event-driven resource watching with:

| Component | File | Purpose |
|-----------|------|---------|
| `types.go` | Event types, interfaces | Core types (EventSender, ResourceEvent, ManagerConfig) |
| `manager.go` | IngestionManager | Coordinates all ingesters, lifecycle management, event processing |
| `pod_ingester.go` | PodIngester | Watches Pod add/update/delete via SharedInformer |
| `service_ingester.go` | ServiceIngester | Watches Service add/update/delete via SharedInformer |
| `namespace_ingester.go` | NamespaceIngester | Watches Namespace add/update/delete via SharedInformer |

## Test Coverage

### Unit Tests (17 tests)

```bash
go test ./internal/ingestion/... -v -run 'Test[^I]'
```

| Test | Description |
|------|-------------|
| `TestNewManager` | Manager creation with default/custom buffer sizes |
| `TestManagerStartStop` | Lifecycle: start, verify running, stop cleanly |
| `TestManagerDoubleStart` | Prevents double-start with error |
| `TestManagerEventProcessing` | End-to-end event processing |
| `TestManagerGetCurrentState` | Snapshot retrieval from informer caches |
| `TestResourceEventStruct` | Event struct field validation |
| `TestPodIngester_OnAdd` | Pod ADD events with correct fields |
| `TestPodIngester_OnUpdate` | Pod UPDATE events on resource changes |
| `TestPodIngester_OnDelete` | Pod DELETE events with nil Object |
| `TestPodIngester_ChannelFull` | Non-blocking behavior when channel full |
| `TestPodIngester_SkipSameResourceVersion` | Deduplication of same-version updates |
| `TestServiceIngester_OnAdd` | Service ADD events |
| `TestServiceIngester_OnUpdate` | Service UPDATE events |
| `TestServiceIngester_OnDelete` | Service DELETE events |
| `TestNamespaceIngester_OnAdd` | Namespace ADD events (cluster-scoped) |
| `TestNamespaceIngester_OnUpdate` | Namespace UPDATE events |
| `TestNamespaceIngester_OnDelete` | Namespace DELETE events |

### Integration Tests (7 tests)

```bash
go test ./internal/ingestion/... -v -run 'TestIntegration'
```

| Test | Description |
|------|-------------|
| `TestIntegration_FullEventFlow` | Complete flow: NS + Pod + Service creation → events sent |
| `TestIntegration_UpdateDeleteFlow` | Update and delete operations produce correct events |
| `TestIntegration_ConcurrentResourceCreation` | 50 concurrent pod creations handled without errors |
| `TestIntegration_EventSenderError` | Manager continues running despite send failures |
| `TestIntegration_GracefulShutdownWithPendingEvents` | Pending events drained on shutdown |
| `TestIntegration_GetCurrentStateAfterSync` | GetCurrentState returns cached resources |
| `TestIntegration_MultipleIngestersNoDeadlock` | No deadlock with concurrent multi-type operations |

## Manual Verification Steps

### 1. Run All Tests

```bash
# Run full test suite
go test ./internal/ingestion/... -v -timeout 120s

# Expected: 24 tests pass (PASS)
# Expected time: ~12-15 seconds
```

### 2. Verify Test Coverage

```bash
# Generate coverage report
go test ./internal/ingestion/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: >80% coverage
```

### 3. Local Development Test (with mock server)

```bash
# 1. Start a mock platform server (using httptest or similar)
# 2. Run the operator locally pointing to mock server
# 3. Create test resources and verify events are sent

# Create test namespace
kubectl create namespace ingestion-test

# Create test pod
kubectl run test-pod --image=nginx -n ingestion-test

# Create test service
kubectl expose pod test-pod --port=80 -n ingestion-test

# Expected: 3 ADD events sent (Namespace, Pod, Service)

# Update pod (add label)
kubectl label pod test-pod app=test -n ingestion-test

# Expected: 1 UPDATE event sent

# Delete resources
kubectl delete ns ingestion-test

# Expected: DELETE events for all resources
```

### 4. Verify Non-Blocking Behavior

```bash
# In a test, simulate a slow/blocked sender
# Verify that:
# - Event channel does not block indefinitely
# - Events are dropped with logged warning when channel is full
# - Manager continues operating normally
```

### 5. Verify Graceful Shutdown

```bash
# 1. Start manager
# 2. Create multiple resources rapidly
# 3. Stop manager immediately
# 4. Verify logs show "drained remaining events"
```

## E2E Test Enhancements

The existing E2E test (`e2e.yml`) should be enhanced to verify delta streaming:

### Proposed E2E Additions

```yaml
# After initial sync verification, test event streaming:

- name: Create test namespace and verify event
  run: |
    kubectl create namespace e2e-delta-test
    sleep 5
    # Verify platform received namespace event
    # (requires platform API to expose received events or logs)

- name: Create test pod and verify event
  run: |
    kubectl run e2e-pod --image=nginx -n e2e-delta-test
    sleep 5
    # Verify platform received pod event

- name: Delete test resources and verify events
  run: |
    kubectl delete namespace e2e-delta-test
    sleep 5
    # Verify platform received delete events
```

## Architecture Verification Checklist

| Requirement | Implementation | Verified By |
|-------------|----------------|-------------|
| SharedInformerFactory pattern | `manager.go:94` creates factory | Unit tests |
| Event buffering | Channel with configurable size | `TestNewManager` |
| Non-blocking sends | Select with default case | `TestPodIngester_ChannelFull` |
| Resource version deduplication | Check in `onUpdate` | `TestPodIngester_SkipSameResourceVersion` |
| Graceful shutdown | drainEvents with timeout | `TestIntegration_GracefulShutdownWithPendingEvents` |
| Error resilience | Log and continue on send errors | `TestIntegration_EventSenderError` |
| Cache sync before processing | WaitForCacheSync | `TestManagerStartStop` |
| Concurrent access safety | Mutex on manager state | `TestIntegration_ConcurrentResourceCreation` |

## Log Verification Points

When testing manually or in E2E, look for these log entries:

### Startup Logs
```
INFO  ingestion-manager  starting ingestion manager  {"clusterUID": "..."}
INFO  ingestion-manager  waiting for informer caches to sync
INFO  ingestion-manager  informer caches synced successfully
DEBUG ingestion-manager  event processor started
```

### Event Processing Logs
```
DEBUG ingestion-manager  event sent successfully  {"type": "ADDED", "kind": "Pod", "name": "...", "namespace": "..."}
DEBUG ingestion-manager  event sent successfully  {"type": "UPDATED", "kind": "Service", ...}
DEBUG ingestion-manager  event sent successfully  {"type": "DELETED", "kind": "Namespace", ...}
```

### Error Logs (should not crash)
```
ERROR ingestion-manager  failed to send event  {"type": "...", "kind": "...", "error": "..."}
ERROR pod-ingester       event channel full, dropping event  {"type": "...", "name": "..."}
```

### Shutdown Logs
```
INFO  ingestion-manager  stopping ingestion manager
INFO  ingestion-manager  drained remaining events  {"count": N}
DEBUG ingestion-manager  event processor stopped
INFO  ingestion-manager  ingestion manager stopped
```

## Success Criteria

The implementation is considered complete when:

1. **All 24 tests pass** consistently
2. **Coverage >80%** on ingestion package
3. **No race conditions** detected with `-race` flag
4. **E2E test** verifies events reach the platform
5. **Manual verification** confirms correct event types and payloads

## Running the Complete Test Suite

```bash
# Full verification command
cd /Users/vinodgupta/workspace/obsyk/obsyk-operator

# 1. Unit + Integration tests with race detection
go test ./internal/ingestion/... -v -race -timeout 120s

# 2. Coverage report
go test ./internal/ingestion/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# 3. All package tests
go test ./... -v -timeout 300s

# 4. CI simulation
earthly +ci
```

## Next Steps for Full Integration

The ingestion package is complete and tested. To fully integrate with the operator:

1. **Wire up in main.go**: Create IngestionManager and start it with controller context
2. **Connect to reconciler**: Use transport.Client as EventSender in ManagerConfig
3. **E2E verification**: Enhance e2e.yml to verify event streaming end-to-end

## Files Changed

```
internal/ingestion/
├── types.go                    # Event types and interfaces
├── manager.go                  # IngestionManager implementation
├── pod_ingester.go             # Pod SharedInformer integration
├── service_ingester.go         # Service SharedInformer integration
├── namespace_ingester.go       # Namespace SharedInformer integration
├── manager_test.go             # Manager unit tests
├── pod_ingester_test.go        # Pod ingester unit tests
├── service_ingester_test.go    # Service ingester unit tests
├── namespace_ingester_test.go  # Namespace ingester unit tests
└── integration_test.go         # Integration tests
```
