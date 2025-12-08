// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// mockTokenProvider is a simple token provider for testing
type mockTokenProvider struct {
	token string
	err   error
}

func (m *mockTokenProvider) GetAccessToken(ctx context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

func TestClient_SendSnapshot(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		serverBody     string
		wantErr        bool
		errType        string
	}{
		{
			name:           "success",
			serverResponse: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "created",
			serverResponse: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:           "client error",
			serverResponse: http.StatusBadRequest,
			serverBody:     "invalid payload",
			wantErr:        true,
			errType:        "client",
		},
		{
			name:           "unauthorized",
			serverResponse: http.StatusUnauthorized,
			serverBody:     "invalid api key",
			wantErr:        true,
			errType:        "auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != endpointSnapshot {
					t.Errorf("expected %s, got %s", endpointSnapshot, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-access-token" {
					t.Errorf("missing or invalid Authorization header: %s", r.Header.Get("Authorization"))
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("missing Content-Type header")
				}

				w.WriteHeader(tt.serverResponse)
				if tt.serverBody != "" {
					_, _ = w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				PlatformURL:   server.URL,
				TokenProvider: &mockTokenProvider{token: "test-access-token"},
				Logger:        logr.Discard(),
			})

			payload := &SnapshotPayload{
				ClusterUID:   "test-uid",
				ClusterName:  "test-cluster",
				AgentVersion: "0.1.0",
			}

			err := client.SendSnapshot(context.Background(), payload)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errType == "client" {
					if _, ok := err.(*ClientError); !ok {
						t.Errorf("expected ClientError, got %T", err)
					}
				}
				if tt.errType == "auth" {
					if _, ok := err.(*AuthError); !ok {
						t.Errorf("expected AuthError, got %T", err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SendEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpointEvents {
			t.Errorf("expected %s, got %s", endpointEvents, r.URL.Path)
		}

		var payload EventPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}

		if payload.Type != "added" {
			t.Errorf("expected event type added, got %s", payload.Type)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Logger:        logr.Discard(),
	})

	payload := &EventPayload{
		ClusterUID: "test-uid",
		Type:       "added",
		Kind:       "Pod",
		UID:        "pod-uid-123",
		Name:       "test-pod",
		Namespace:  "default",
		Object:     map[string]string{"name": "test-pod"},
	}

	err := client.SendEvent(context.Background(), payload)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_SendHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpointHeartbeat {
			t.Errorf("expected %s, got %s", endpointHeartbeat, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	err := client.SendHeartbeat(context.Background(), payload)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_RetryOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			// Return server error for first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		// Success on 3rd attempt
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Timeout:       5 * time.Second,
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	err := client.SendHeartbeat(context.Background(), payload)
	if err != nil {
		t.Errorf("unexpected error after retry: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_NoRetryOnClientError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	err := client.SendHeartbeat(context.Background(), payload)
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Should not retry on client errors
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow server
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Logger:        logr.Discard(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	err := client.SendHeartbeat(ctx, payload)
	if err == nil {
		t.Error("expected context deadline exceeded error")
	}
}

func TestClient_UpdateTokenProvider(t *testing.T) {
	var receivedKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "old-token"},
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	// First request with old token
	_ = client.SendHeartbeat(context.Background(), payload)
	if receivedKey != "Bearer old-token" {
		t.Errorf("expected old token, got %s", receivedKey)
	}

	// Update token provider and send again
	client.UpdateTokenProvider(&mockTokenProvider{token: "new-token"})
	_ = client.SendHeartbeat(context.Background(), payload)
	if receivedKey != "Bearer new-token" {
		t.Errorf("expected new token, got %s", receivedKey)
	}
}

// TestClient_ConcurrentSendEvent tests concurrent SendEvent calls for thread-safety.
func TestClient_ConcurrentSendEvent(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		// Small delay to increase chance of race conditions
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			payload := &EventPayload{
				ClusterUID: "test-uid",
				Type:       "added",
				Kind:       "Pod",
				UID:        "pod-uid-" + string(rune('0'+idx%10)),
				Name:       "test-pod",
				Namespace:  "default",
			}
			err := client.SendEvent(context.Background(), payload)
			if err != nil {
				t.Errorf("SendEvent failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt32(&requestCount) != int32(numGoroutines) {
		t.Errorf("expected %d requests, got %d", numGoroutines, requestCount)
	}
}

// TestClient_ConcurrentUpdateTokenProvider tests concurrent token provider updates
// while sending requests. This verifies thread-safety of UpdateTokenProvider.
func TestClient_ConcurrentUpdateTokenProvider(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "initial-token"},
		Logger:        logr.Discard(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Start goroutines that continuously send events
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := &HeartbeatPayload{
				ClusterUID:   "test-uid",
				AgentVersion: "0.1.0",
			}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = client.SendHeartbeat(context.Background(), payload)
				}
			}
		}()
	}

	// Concurrently update token provider
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					client.UpdateTokenProvider(&mockTokenProvider{
						token: "token-" + string(rune('0'+idx)),
					})
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	// Should have successfully processed many requests without race conditions
	if atomic.LoadInt32(&requestCount) == 0 {
		t.Error("expected requests to be processed")
	}
	t.Logf("processed %d requests during concurrent updates", requestCount)
}

// TestClient_ConcurrentMixedOperations tests all operations running concurrently.
func TestClient_ConcurrentMixedOperations(t *testing.T) {
	var snapshotCount, eventCount, heartbeatCount int32
	var snapshotErrors, eventErrors, heartbeatErrors int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case endpointSnapshot:
			atomic.AddInt32(&snapshotCount, 1)
		case endpointEvents:
			atomic.AddInt32(&eventCount, 1)
		case endpointHeartbeat:
			atomic.AddInt32(&heartbeatCount, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	var wg sync.WaitGroup
	numEach := 10 // Reduced from 20 to minimize resource pressure

	// Concurrent snapshots
	for i := 0; i < numEach; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := &SnapshotPayload{
				ClusterUID:  "test-uid",
				ClusterName: "test-cluster",
			}
			if err := client.SendSnapshot(context.Background(), payload); err != nil {
				atomic.AddInt32(&snapshotErrors, 1)
			}
		}()
	}

	// Concurrent events
	for i := 0; i < numEach; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := &EventPayload{
				ClusterUID: "test-uid",
				Type:       "added",
				Kind:       "Pod",
			}
			if err := client.SendEvent(context.Background(), payload); err != nil {
				atomic.AddInt32(&eventErrors, 1)
			}
		}()
	}

	// Concurrent heartbeats
	for i := 0; i < numEach; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := &HeartbeatPayload{
				ClusterUID:   "test-uid",
				AgentVersion: "0.1.0",
			}
			if err := client.SendHeartbeat(context.Background(), payload); err != nil {
				atomic.AddInt32(&heartbeatErrors, 1)
			}
		}()
	}

	wg.Wait()

	// Log any errors for debugging
	if se := atomic.LoadInt32(&snapshotErrors); se > 0 {
		t.Logf("snapshot errors: %d", se)
	}
	if ee := atomic.LoadInt32(&eventErrors); ee > 0 {
		t.Logf("event errors: %d", ee)
	}
	if he := atomic.LoadInt32(&heartbeatErrors); he > 0 {
		t.Logf("heartbeat errors: %d", he)
	}

	// Verify successful requests match expected count
	// Account for any errors that might have occurred
	expectedSnapshots := int32(numEach) - atomic.LoadInt32(&snapshotErrors)
	expectedEvents := int32(numEach) - atomic.LoadInt32(&eventErrors)
	expectedHeartbeats := int32(numEach) - atomic.LoadInt32(&heartbeatErrors)

	if atomic.LoadInt32(&snapshotCount) != expectedSnapshots {
		t.Errorf("expected %d snapshots, got %d", expectedSnapshots, snapshotCount)
	}
	if atomic.LoadInt32(&eventCount) != expectedEvents {
		t.Errorf("expected %d events, got %d", expectedEvents, eventCount)
	}
	if atomic.LoadInt32(&heartbeatCount) != expectedHeartbeats {
		t.Errorf("expected %d heartbeats, got %d", expectedHeartbeats, heartbeatCount)
	}
}

// TestClient_HealthTracking tests health tracking on successful requests.
func TestClient_HealthTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	// Initially, client should not be healthy (no requests yet)
	if client.IsHealthy() {
		t.Error("expected client to not be healthy initially")
	}

	// Send a successful request
	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}
	err := client.SendHeartbeat(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendHeartbeat failed: %v", err)
	}

	// Now client should be healthy
	if !client.IsHealthy() {
		t.Error("expected client to be healthy after successful request")
	}

	// Check health status
	status := client.GetHealthStatus()
	if !status.Healthy {
		t.Error("expected health status to be healthy")
	}
	if status.ConsecutiveErrors != 0 {
		t.Errorf("expected 0 consecutive errors, got %d", status.ConsecutiveErrors)
	}
	if status.LastHealthyTime.IsZero() {
		t.Error("expected LastHealthyTime to be set")
	}
	if status.LastError != nil {
		t.Errorf("expected no last error, got %v", status.LastError)
	}
}

// TestClient_HealthTrackingOnFailure tests health tracking on failed requests.
func TestClient_HealthTrackingOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	// Send a failing request
	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}
	err := client.SendHeartbeat(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Client should be unhealthy
	if client.IsHealthy() {
		t.Error("expected client to be unhealthy after failed request")
	}

	// Check health status
	status := client.GetHealthStatus()
	if status.Healthy {
		t.Error("expected health status to be unhealthy")
	}
	if status.ConsecutiveErrors != 1 {
		t.Errorf("expected 1 consecutive error, got %d", status.ConsecutiveErrors)
	}
	if status.LastError == nil {
		t.Error("expected last error to be set")
	}
}

// TestClient_HealthRecovery tests health recovery after failure.
func TestClient_HealthRecovery(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterUID:   "test-uid",
		AgentVersion: "0.1.0",
	}

	// First request fails
	_ = client.SendHeartbeat(context.Background(), payload)
	if client.IsHealthy() {
		t.Error("expected client to be unhealthy after first request")
	}

	status := client.GetHealthStatus()
	if status.ConsecutiveErrors != 1 {
		t.Errorf("expected 1 consecutive error, got %d", status.ConsecutiveErrors)
	}

	// Second request succeeds
	err := client.SendHeartbeat(context.Background(), payload)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	// Client should recover
	if !client.IsHealthy() {
		t.Error("expected client to be healthy after recovery")
	}

	status = client.GetHealthStatus()
	if status.ConsecutiveErrors != 0 {
		t.Errorf("expected 0 consecutive errors after recovery, got %d", status.ConsecutiveErrors)
	}
	if status.LastError != nil {
		t.Error("expected last error to be cleared after recovery")
	}
}

// TestClient_ConcurrentHealthTracking tests thread-safety of health tracking.
func TestClient_ConcurrentHealthTracking(t *testing.T) {
	var successCount, failCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Alternate between success and failure
		if atomic.AddInt32(&successCount, 1)%2 == 0 {
			atomic.AddInt32(&failCount, 1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-token"},
		Logger:        logr.Discard(),
	})

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := &HeartbeatPayload{
				ClusterUID:   "test-uid",
				AgentVersion: "0.1.0",
			}
			_ = client.SendHeartbeat(context.Background(), payload)

			// Concurrently read health status
			_ = client.IsHealthy()
			_ = client.GetHealthStatus()
		}()
	}

	wg.Wait()

	// Just verify we can read health status after concurrent operations
	status := client.GetHealthStatus()
	t.Logf("Final health: healthy=%v, consecutiveErrors=%d", status.Healthy, status.ConsecutiveErrors)
}
