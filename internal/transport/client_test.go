// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
					w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				PlatformURL:   server.URL,
				TokenProvider: &mockTokenProvider{token: "test-access-token"},
				Logger:        logr.Discard(),
			})

			payload := &SnapshotPayload{
				ClusterName: "test-cluster",
				ClusterUID:  "test-uid",
				Timestamp:   time.Now(),
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

		if payload.EventType != EventTypeAdded {
			t.Errorf("expected event type ADDED, got %s", payload.EventType)
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
		ClusterName:  "test-cluster",
		ClusterUID:   "test-uid",
		Timestamp:    time.Now(),
		EventType:    EventTypeAdded,
		ResourceType: ResourceTypePod,
		Resource:     map[string]string{"name": "test-pod"},
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
		ClusterName: "test-cluster",
		ClusterUID:  "test-uid",
		Timestamp:   time.Now(),
		ResourceCounts: ResourceCounts{
			Namespaces: 10,
			Pods:       100,
			Services:   20,
		},
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
			w.Write([]byte("server error"))
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
		ClusterName: "test-cluster",
		ClusterUID:  "test-uid",
		Timestamp:   time.Now(),
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
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		PlatformURL:   server.URL,
		TokenProvider: &mockTokenProvider{token: "test-access-token"},
		Logger:        logr.Discard(),
	})

	payload := &HeartbeatPayload{
		ClusterName: "test-cluster",
		ClusterUID:  "test-uid",
		Timestamp:   time.Now(),
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
		ClusterName: "test-cluster",
		ClusterUID:  "test-uid",
		Timestamp:   time.Now(),
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
		ClusterName: "test-cluster",
		ClusterUID:  "test-uid",
		Timestamp:   time.Now(),
	}

	// First request with old token
	client.SendHeartbeat(context.Background(), payload)
	if receivedKey != "Bearer old-token" {
		t.Errorf("expected old token, got %s", receivedKey)
	}

	// Update token provider and send again
	client.UpdateTokenProvider(&mockTokenProvider{token: "new-token"})
	client.SendHeartbeat(context.Background(), payload)
	if receivedKey != "Bearer new-token" {
		t.Errorf("expected new token, got %s", receivedKey)
	}
}
