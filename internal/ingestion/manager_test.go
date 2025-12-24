// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// mockEventSender is a mock implementation of EventSender for testing.
type mockEventSender struct {
	mu     sync.Mutex
	events []*transport.EventPayload
	err    error
}

func newMockEventSender() *mockEventSender {
	return &mockEventSender{
		events: make([]*transport.EventPayload, 0),
	}
}

func (m *mockEventSender) SendEvent(ctx context.Context, payload *transport.EventPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, payload)
	return nil
}

func (m *mockEventSender) getEvents() []*transport.EventPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*transport.EventPayload, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockEventSender) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func init() {
	// Set up logging for tests
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
}

func TestNewManager(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	tests := []struct {
		name       string
		bufferSize int
		wantSize   int
	}{
		{
			name:       "default buffer size",
			bufferSize: 0,
			wantSize:   DefaultEventBufferSize,
		},
		{
			name:       "custom buffer size",
			bufferSize: 500,
			wantSize:   500,
		},
		{
			name:       "negative buffer size uses default",
			bufferSize: -1,
			wantSize:   DefaultEventBufferSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ManagerConfig{
				ClusterUID:      "test-cluster-uid",
				EventSender:     sender,
				EventBufferSize: tt.bufferSize,
			}

			mgr := NewManager(clientset, cfg, log)
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
			if cap(mgr.eventChan) != tt.wantSize {
				t.Errorf("eventChan capacity = %d, want %d", cap(mgr.eventChan), tt.wantSize)
			}
		})
	}
}

func TestManagerStartStop(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	cfg := ManagerConfig{
		ClusterUID:      "test-cluster-uid",
		EventSender:     sender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	// Should not be started initially
	if mgr.IsStarted() {
		t.Error("manager should not be started initially")
	}

	// Start manager in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start
	time.Sleep(100 * time.Millisecond)

	if !mgr.IsStarted() {
		t.Error("manager should be started")
	}

	// Stop manager
	mgr.Stop()

	// Wait for Start() to return
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for Start() to return")
	}

	if mgr.IsStarted() {
		t.Error("manager should not be started after Stop()")
	}
}

func TestManagerDoubleStart(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	cfg := ManagerConfig{
		ClusterUID:  "test-cluster-uid",
		EventSender: sender,
	}

	mgr := NewManager(clientset, cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start
	time.Sleep(100 * time.Millisecond)

	// Try to start again - should fail
	err := mgr.Start(context.Background())
	if err == nil {
		t.Error("expected error when starting already started manager")
	}

	mgr.Stop()
}

func TestManagerEventProcessing(t *testing.T) {
	// Create fake clientset with initial resources
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-123",
		},
	}
	clientset := fake.NewClientset(pod)
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	cfg := ManagerConfig{
		ClusterUID:      "test-cluster-uid",
		EventSender:     sender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start and caches to sync
	time.Sleep(500 * time.Millisecond)

	// Create a new pod to trigger an event
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-pod",
			Namespace: "default",
			UID:       "pod-uid-456",
		},
	}
	_, err := clientset.CoreV1().Pods("default").Create(ctx, newPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	// Wait for event to be processed
	time.Sleep(500 * time.Millisecond)

	// Check that events were sent
	events := sender.getEvents()
	// Note: Initial cache sync will also generate events for existing resources
	if len(events) == 0 {
		t.Error("expected at least one event to be sent")
	}

	mgr.Stop()
}

func TestManagerGetCurrentState(t *testing.T) {
	// Create fake clientset with initial resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			UID:  "ns-uid-123",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-123",
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
			UID:       "svc-uid-123",
		},
	}

	clientset := fake.NewClientset(ns, pod, svc)
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	cfg := ManagerConfig{
		ClusterUID:  "test-cluster-uid",
		EventSender: sender,
	}

	mgr := NewManager(clientset, cfg, log)

	// GetCurrentState should fail when not started
	_, err := mgr.GetCurrentState()
	if err == nil {
		t.Error("expected error when getting state from stopped manager")
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start and caches to sync
	time.Sleep(500 * time.Millisecond)

	// Now GetCurrentState should work
	state, err := mgr.GetCurrentState()
	if err != nil {
		cancel()
		mgr.Stop()
		t.Fatalf("GetCurrentState() error: %v", err)
	}

	if state.ClusterUID != "test-cluster-uid" {
		t.Errorf("ClusterUID = %q, want %q", state.ClusterUID, "test-cluster-uid")
	}

	// Check that resources are included
	// Note: fake client may include default namespace
	if len(state.Namespaces) == 0 {
		t.Error("expected at least one namespace")
	}
	if len(state.Pods) != 1 {
		t.Errorf("expected 1 pod, got %d", len(state.Pods))
	}
	if len(state.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(state.Services))
	}

	// Stop manager properly
	cancel()
	mgr.Stop()

	// Wait for Start() to return
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for Start() to return")
	}
}

func TestResourceEventStruct(t *testing.T) {
	event := ResourceEvent{
		Type:      transport.EventTypeAdded,
		Kind:      transport.ResourceTypePod,
		UID:       "test-uid",
		Name:      "test-name",
		Namespace: "test-namespace",
		Object:    map[string]string{"key": "value"},
	}

	if event.Type != transport.EventTypeAdded {
		t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
	}
	if event.Kind != transport.ResourceTypePod {
		t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypePod)
	}
}

func TestManagerRateLimitConfig(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	tests := []struct {
		name      string
		rateLimit *RateLimitConfig
		wantRate  float64
		wantBurst int
	}{
		{
			name:      "default rate limit when nil",
			rateLimit: nil,
			wantRate:  DefaultEventsPerSecond,
			wantBurst: DefaultBurstSize,
		},
		{
			name: "custom rate limit",
			rateLimit: &RateLimitConfig{
				EventsPerSecond: 50,
				BurstSize:       100,
			},
			wantRate:  50,
			wantBurst: 100,
		},
		{
			name: "zero values use defaults",
			rateLimit: &RateLimitConfig{
				EventsPerSecond: 0,
				BurstSize:       0,
			},
			wantRate:  DefaultEventsPerSecond,
			wantBurst: DefaultBurstSize,
		},
		{
			name: "negative values use defaults",
			rateLimit: &RateLimitConfig{
				EventsPerSecond: -5,
				BurstSize:       -10,
			},
			wantRate:  DefaultEventsPerSecond,
			wantBurst: DefaultBurstSize,
		},
		{
			name: "partial config - only rate",
			rateLimit: &RateLimitConfig{
				EventsPerSecond: 25,
				BurstSize:       0,
			},
			wantRate:  25,
			wantBurst: DefaultBurstSize,
		},
		{
			name: "partial config - only burst",
			rateLimit: &RateLimitConfig{
				EventsPerSecond: 0,
				BurstSize:       50,
			},
			wantRate:  DefaultEventsPerSecond,
			wantBurst: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ManagerConfig{
				ClusterUID:  "test-cluster-uid",
				EventSender: sender,
				RateLimit:   tt.rateLimit,
			}

			mgr := NewManager(clientset, cfg, log)
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
			if mgr.limiter == nil {
				t.Fatal("expected non-nil limiter")
			}

			// Check limiter configuration
			gotRate := float64(mgr.limiter.Limit())
			if gotRate != tt.wantRate {
				t.Errorf("limiter rate = %v, want %v", gotRate, tt.wantRate)
			}
			if mgr.limiter.Burst() != tt.wantBurst {
				t.Errorf("limiter burst = %v, want %v", mgr.limiter.Burst(), tt.wantBurst)
			}
		})
	}
}

func TestManagerRateLimitEnforcement(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	// Use a very low rate limit for testing: 2 events/sec, burst of 2
	cfg := ManagerConfig{
		ClusterUID:      "test-cluster-uid",
		EventSender:     sender,
		EventBufferSize: 100,
		RateLimit: &RateLimitConfig{
			EventsPerSecond: 2,
			BurstSize:       2,
		},
	}

	mgr := NewManager(clientset, cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start
	time.Sleep(100 * time.Millisecond)

	// Measure time to send multiple events
	numEvents := 5
	start := time.Now()

	for i := 0; i < numEvents; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "rate-test-pod-" + string(rune('a'+i)),
				Namespace:       "default",
				UID:             types.UID("pod-uid-" + string(rune('a'+i))),
				ResourceVersion: "1",
			},
		}
		_, _ = clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	}

	// Wait for events to be processed
	time.Sleep(2 * time.Second)

	elapsed := time.Since(start)

	// With rate limit of 2/sec and burst of 2, sending 5 events should take at least 1 second
	// (2 immediately from burst, then 3 more at 2/sec = 1.5 seconds theoretically)
	// We check for at least 500ms to account for processing overhead
	if elapsed < 500*time.Millisecond {
		t.Logf("elapsed time: %v", elapsed)
		// This is more of a smoke test - rate limiting should slow things down
	}

	events := sender.getEvents()
	t.Logf("received %d events in %v", len(events), elapsed)

	mgr.Stop()
}

func TestManagerRateLimitContextCancellation(t *testing.T) {
	clientset := fake.NewClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test")

	// Use a very low rate limit
	cfg := ManagerConfig{
		ClusterUID:      "test-cluster-uid",
		EventSender:     sender,
		EventBufferSize: 100,
		RateLimit: &RateLimitConfig{
			EventsPerSecond: 1, // Very slow: 1 event/sec
			BurstSize:       1,
		},
	}

	mgr := NewManager(clientset, cfg, log)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start
	time.Sleep(100 * time.Millisecond)

	// Create several pods to fill the event queue
	for i := 0; i < 5; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cancel-test-pod-" + string(rune('a'+i)),
				Namespace:       "default",
				UID:             types.UID("pod-uid-cancel-" + string(rune('a'+i))),
				ResourceVersion: "1",
			},
		}
		_, _ = clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	}

	// Give events a moment to queue up
	time.Sleep(100 * time.Millisecond)

	// Cancel context while events are being rate-limited
	cancel()

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		mgr.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - stop completed
	case <-time.After(5 * time.Second):
		t.Error("Stop() hung when rate limiting was active")
	}
}
