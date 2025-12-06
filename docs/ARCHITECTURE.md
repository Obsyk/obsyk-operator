# Obsyk Operator Architecture

This document describes the architecture of the Obsyk Operator, a Kubernetes controller that streams cluster metadata to the Obsyk platform.

## Overview

The operator follows the standard Kubernetes controller pattern, enhanced with event-driven delta streaming for real-time resource synchronization.

## System Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌───────────────────────────────────────────────────────────────────────┐ │
│   │                        obsyk-operator Pod                              │ │
│   │                                                                        │ │
│   │  ┌─────────────────────────────────────────────────────────────────┐  │ │
│   │  │                  ObsykAgentReconciler                            │  │ │
│   │  │                                                                  │  │ │
│   │  │  Responsibilities:                                               │  │ │
│   │  │  - Watch ObsykAgent CR for changes                              │  │ │
│   │  │  - Manage agent lifecycle                                        │  │ │
│   │  │  - Send initial snapshot on startup                              │  │ │
│   │  │  - Send periodic heartbeats                                      │  │ │
│   │  │  - Update status conditions                                      │  │ │
│   │  │                                                                  │  │ │
│   │  │  Thread-Safety:                                                  │  │ │
│   │  │  - agentClientsMu sync.RWMutex protects agentClients map        │  │ │
│   │  │  - Double-checked locking for getOrCreateAgentClient()          │  │ │
│   │  └─────────────────────────────────────────────────────────────────┘  │ │
│   │                               │                                        │ │
│   │                               ▼                                        │ │
│   │  ┌─────────────────────────────────────────────────────────────────┐  │ │
│   │  │                   IngestionManager                               │  │ │
│   │  │                                                                  │  │ │
│   │  │  ┌─────────────────────────────────────────────────────────┐    │  │ │
│   │  │  │              SharedInformerFactory                       │    │  │ │
│   │  │  │                                                          │    │  │ │
│   │  │  │   Watches:                                               │    │  │ │
│   │  │  │   - Pods (all namespaces)                                │    │  │ │
│   │  │  │   - Services (all namespaces)                            │    │  │ │
│   │  │  │   - Namespaces (cluster-scoped)                          │    │  │ │
│   │  │  │                                                          │    │  │ │
│   │  │  │   Features:                                              │    │  │ │
│   │  │  │   - Shared cache across all ingesters                    │    │  │ │
│   │  │  │   - Efficient watch multiplexing                         │    │  │ │
│   │  │  │   - No periodic resync (event-only)                      │    │  │ │
│   │  │  └─────────────────────────────────────────────────────────┘    │  │ │
│   │  │                               │                                  │  │ │
│   │  │           ┌───────────────────┼───────────────────┐              │  │ │
│   │  │           ▼                   ▼                   ▼              │  │ │
│   │  │  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐        │  │ │
│   │  │  │PodIngester  │     │SvcIngester  │     │NSIngester   │        │  │ │
│   │  │  │             │     │             │     │             │        │  │ │
│   │  │  │ OnAdd()     │     │ OnAdd()     │     │ OnAdd()     │        │  │ │
│   │  │  │ OnUpdate()  │     │ OnUpdate()  │     │ OnUpdate()  │        │  │ │
│   │  │  │ OnDelete()  │     │ OnDelete()  │     │ OnDelete()  │        │  │ │
│   │  │  └──────┬──────┘     └──────┬──────┘     └──────┬──────┘        │  │ │
│   │  │         │                   │                   │                │  │ │
│   │  │         └───────────────────┼───────────────────┘                │  │ │
│   │  │                             ▼                                    │  │ │
│   │  │  ┌─────────────────────────────────────────────────────────┐    │  │ │
│   │  │  │            Event Channel (buffered: 1000)                │    │  │ │
│   │  │  │                                                          │    │  │ │
│   │  │  │   ResourceEvent{Type, Kind, UID, Name, Namespace, Obj}  │    │  │ │
│   │  │  └─────────────────────────────────────────────────────────┘    │  │ │
│   │  │                             │                                    │  │ │
│   │  │                             ▼                                    │  │ │
│   │  │  ┌─────────────────────────────────────────────────────────┐    │  │ │
│   │  │  │              Event Processor (goroutine)                 │    │  │ │
│   │  │  │                                                          │    │  │ │
│   │  │  │   - Reads events from channel                            │    │  │ │
│   │  │  │   - Converts to transport.EventPayload                   │    │  │ │
│   │  │  │   - Sends via transport.Client                           │    │  │ │
│   │  │  │   - Handles errors with logging (continues operation)    │    │  │ │
│   │  │  └─────────────────────────────────────────────────────────┘    │  │ │
│   │  └─────────────────────────────────────────────────────────────────┘  │ │
│   │                               │                                        │ │
│   │                               ▼                                        │ │
│   │  ┌─────────────────────────────────────────────────────────────────┐  │ │
│   │  │                    transport.Client                              │  │ │
│   │  │                                                                  │  │ │
│   │  │  Methods:                                                        │  │ │
│   │  │  - SendSnapshot(ctx, payload) - Full cluster state              │  │ │
│   │  │  - SendEvent(ctx, payload) - Single resource change             │  │ │
│   │  │  - SendHeartbeat(ctx, payload) - Health check                   │  │ │
│   │  │                                                                  │  │ │
│   │  │  Features:                                                       │  │ │
│   │  │  - Retry with exponential backoff (5xx only)                    │  │ │
│   │  │  - TokenProvider for OAuth2 Bearer tokens                       │  │ │
│   │  │  - Thread-safe tokenProvider updates                            │  │ │
│   │  └─────────────────────────────────────────────────────────────────┘  │ │
│   │                               │                                        │ │
│   │                               ▼                                        │ │
│   │  ┌─────────────────────────────────────────────────────────────────┐  │ │
│   │  │                    auth.TokenManager                             │  │ │
│   │  │                                                                  │  │ │
│   │  │  - Creates JWT assertions (ES256, ECDSA P-256)                  │  │ │
│   │  │  - Exchanges for access tokens via /oauth/token                 │  │ │
│   │  │  - Caches tokens, refreshes before expiry                       │  │ │
│   │  │  - Thread-safe credential updates                               │  │ │
│   │  └─────────────────────────────────────────────────────────────────┘  │ │
│   └───────────────────────────────────────────────────────────────────────┘ │
│                                    │                                         │
└────────────────────────────────────│─────────────────────────────────────────┘
                                     │ HTTPS
                                     ▼
                      ┌──────────────────────────────┐
                      │       Obsyk Platform         │
                      │                              │
                      │  POST /oauth/token           │
                      │  POST /api/v1/agent/snapshot │
                      │  POST /api/v1/agent/events   │
                      │  POST /api/v1/agent/heartbeat│
                      └──────────────────────────────┘
```

## Data Flow

### 1. Startup Sequence

```
1. Operator starts
   └── Controller manager initializes
       └── ObsykAgentReconciler registered

2. ObsykAgent CR detected
   └── Reconcile() called
       ├── Fetch credentials from Secret
       ├── Create TokenManager
       ├── Create transport.Client
       ├── Get cluster UID from kube-system namespace
       └── Send initial snapshot

3. IngestionManager started
   └── SharedInformerFactory created
       ├── Pod informer registered
       ├── Service informer registered
       ├── Namespace informer registered
       └── WaitForCacheSync()

4. Event processing begins
   └── Goroutine reads from event channel
       └── Sends events to platform
```

### 2. Event Processing Flow

```
K8s API Server
     │
     │ Watch stream (Pod/Service/Namespace)
     ▼
SharedInformer
     │
     │ OnAdd/OnUpdate/OnDelete callback
     ▼
Ingester (Pod/Service/Namespace)
     │
     │ Non-blocking send to channel
     │ (drops if channel full, logs warning)
     ▼
Event Channel (buffered, 1000 capacity)
     │
     │ Event processor goroutine reads
     ▼
transport.Client.SendEvent()
     │
     │ HTTP POST with retry
     ▼
Obsyk Platform
```

### 3. Authentication Flow

```
1. Credentials loaded from Secret
   ├── client_id: OAuth2 client ID
   └── private_key: ECDSA P-256 PEM

2. JWT assertion created
   ├── iss: client_id
   ├── sub: client_id
   ├── aud: platform URL
   ├── iat: current time
   ├── exp: +5 minutes
   ├── jti: unique ID
   └── Signed with ES256 (private key)

3. Token exchange
   └── POST /oauth/token
       ├── grant_type: jwt-bearer
       └── assertion: signed JWT

4. Access token received
   └── Cached with expiry tracking
       └── Refreshed 5 minutes before expiry

5. API calls use Bearer token
   └── Authorization: Bearer <access_token>
```

## Thread-Safety Model

All major components are designed for concurrent access:

| Component | Shared State | Protection | Pattern |
|-----------|--------------|------------|---------|
| ObsykAgentReconciler | `agentClients` map | `sync.RWMutex` | Double-checked locking |
| transport.Client | `tokenProvider` | `sync.RWMutex` | Lock on update, RLock on read |
| auth.TokenManager | `accessToken`, `credentials` | `sync.RWMutex` | Lock for refresh, RLock for get |
| IngestionManager | `started` flag | `sync.RWMutex` | Lock for start/stop |

### Double-Checked Locking Pattern

Used in `getOrCreateAgentClient()` to minimize lock contention:

```go
// Fast path - read lock
r.mu.RLock()
if client, ok := r.clients[key]; ok {
    r.mu.RUnlock()
    return client, nil  // Hit: no write lock needed
}
r.mu.RUnlock()

// Slow path - write lock
r.mu.Lock()
defer r.mu.Unlock()

// Re-check after acquiring write lock
if client, ok := r.clients[key]; ok {
    return client, nil  // Another goroutine created it
}

// Create new client
client := createClient()
r.clients[key] = client
return client, nil
```

## Error Handling

### HTTP Error Responses

| Status | Type | Action |
|--------|------|--------|
| 2xx | Success | Continue |
| 401 | Auth error | Refresh token, retry once |
| 4xx | Client error | Log, don't retry |
| 5xx | Server error | Exponential backoff (max 5 retries) |

### Event Channel Full

When the event channel is full, ingesters:
1. Log a warning with event details
2. Drop the event (non-blocking)
3. Continue processing new events

This prevents slow consumers from blocking Kubernetes API watches.

## Resource Requirements

### RBAC Permissions

The operator requires read-only access:

```yaml
rules:
  - apiGroups: [""]
    resources: ["namespaces", "pods", "services"]
    verbs: ["list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]  # For credentials
  - apiGroups: ["obsyk.io"]
    resources: ["obsykagents"]
    verbs: ["get", "list", "watch", "update", "patch"]  # For status
```

### Memory Footprint

- Informer cache: Proportional to cluster size (pods, services, namespaces)
- Event buffer: 1000 events * ~1KB = ~1MB
- Token cache: Minimal (~1KB)

Recommended limits:
- Small clusters (<1000 pods): 128Mi
- Medium clusters (1000-10000 pods): 256Mi
- Large clusters (>10000 pods): 512Mi+

## Security Considerations

1. **Credentials**: Never stored in CR, only in Secrets
2. **Private keys**: Never logged, short-lived in memory
3. **RBAC**: Minimal read-only permissions
4. **Network**: TLS for all platform communication
5. **Tokens**: Short-lived (1 hour), auto-refresh
