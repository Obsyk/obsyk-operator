// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

// TestIntegration_FullEventFlow tests the complete flow from resource creation to event delivery.
func TestIntegration_FullEventFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientset := fake.NewSimpleClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test-integration")

	cfg := ManagerConfig{
		ClusterUID:      "integration-test-cluster",
		EventSender:     sender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	// Start manager in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	// Wait for manager to start
	time.Sleep(500 * time.Millisecond)

	if !mgr.IsStarted() {
		t.Fatal("manager should be started")
	}

	// Create resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-integration-ns",
			UID:  types.UID("ns-uid-integration"),
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-integration-pod",
			Namespace: "default",
			UID:       types.UID("pod-uid-integration"),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx:latest"},
			},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-integration-svc",
			Namespace: "default",
			UID:       types.UID("svc-uid-integration"),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	// Create all resources
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}
	_, err = clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}
	_, err = clientset.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Wait for events to be processed
	time.Sleep(1 * time.Second)

	// Verify events were sent
	events := sender.getEvents()
	if len(events) < 3 {
		t.Errorf("expected at least 3 events, got %d", len(events))
	}

	// Verify cluster UID is set on all events
	for _, event := range events {
		if event.ClusterUID != "integration-test-cluster" {
			t.Errorf("event ClusterUID = %q, want %q", event.ClusterUID, "integration-test-cluster")
		}
	}

	// Verify we got different kinds
	kindCounts := make(map[string]int)
	for _, event := range events {
		kindCounts[event.Kind]++
	}

	if kindCounts["Namespace"] == 0 {
		t.Error("expected at least one Namespace event")
	}
	if kindCounts["Pod"] == 0 {
		t.Error("expected at least one Pod event")
	}
	if kindCounts["Service"] == 0 {
		t.Error("expected at least one Service event")
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
}

// TestIntegration_UpdateDeleteFlow tests update and delete events are properly processed.
func TestIntegration_UpdateDeleteFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create initial pod
	initialPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "update-delete-pod",
			Namespace:       "default",
			UID:             types.UID("pod-update-delete"),
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(initialPod)
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test-update-delete")

	cfg := ManagerConfig{
		ClusterUID:      "update-delete-cluster",
		EventSender:     sender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Drain initial events
	sender.mu.Lock()
	sender.events = nil
	sender.mu.Unlock()

	// Update the pod
	updatedPod := initialPod.DeepCopy()
	updatedPod.ResourceVersion = "2"
	updatedPod.Labels = map[string]string{"updated": "true"}
	_, err := clientset.CoreV1().Pods("default").Update(ctx, updatedPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update pod: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	events := sender.getEvents()
	foundUpdate := false
	for _, event := range events {
		if event.Type == string(transport.EventTypeUpdated) && event.Kind == string(transport.ResourceTypePod) {
			foundUpdate = true
			break
		}
	}
	if !foundUpdate {
		t.Error("expected update event for pod")
	}

	// Delete the pod
	err = clientset.CoreV1().Pods("default").Delete(ctx, "update-delete-pod", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete pod: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	events = sender.getEvents()
	foundDelete := false
	for _, event := range events {
		if event.Type == string(transport.EventTypeDeleted) && event.Kind == string(transport.ResourceTypePod) {
			foundDelete = true
			// Verify object is nil for delete events
			if event.Object != nil {
				t.Error("Object should be nil for delete event")
			}
			break
		}
	}
	if !foundDelete {
		t.Error("expected delete event for pod")
	}

	mgr.Stop()
	<-errCh
}

// TestIntegration_ConcurrentResourceCreation tests handling of many concurrent resource creations.
func TestIntegration_ConcurrentResourceCreation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientset := fake.NewSimpleClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test-concurrent")

	cfg := ManagerConfig{
		ClusterUID:      "concurrent-cluster",
		EventSender:     sender,
		EventBufferSize: 1000, // Large buffer for concurrent test
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Create many pods concurrently
	numPods := 50
	var wg sync.WaitGroup
	wg.Add(numPods)

	for i := 0; i < numPods; i++ {
		go func(idx int) {
			defer wg.Done()
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "concurrent-pod-" + string(rune('a'+idx%26)) + string(rune('0'+idx/26)),
					Namespace: "default",
					UID:       types.UID("concurrent-uid-" + string(rune('0'+idx))),
				},
			}
			_, err := clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
			if err != nil {
				t.Logf("failed to create pod %d: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all events to be processed
	time.Sleep(2 * time.Second)

	events := sender.getEvents()
	podEvents := 0
	for _, event := range events {
		if event.Kind == string(transport.ResourceTypePod) && event.Type == string(transport.EventTypeAdded) {
			podEvents++
		}
	}

	// Should have received add events for all pods
	if podEvents < numPods {
		t.Errorf("expected at least %d pod add events, got %d", numPods, podEvents)
	}

	mgr.Stop()
	<-errCh
}

// TestIntegration_EventSenderError tests behavior when event sender returns errors.
func TestIntegration_EventSenderError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientset := fake.NewSimpleClientset()
	sender := newMockEventSender()
	sender.setError(context.DeadlineExceeded)
	log := ctrl.Log.WithName("test-sender-error")

	cfg := ManagerConfig{
		ClusterUID:      "error-cluster",
		EventSender:     sender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Create a pod - should not crash even though sender returns error
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-pod",
			Namespace: "default",
			UID:       types.UID("error-pod-uid"),
		},
	}
	_, err := clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	// Wait for event processing attempt
	time.Sleep(500 * time.Millisecond)

	// Manager should still be running
	if !mgr.IsStarted() {
		t.Error("manager should still be running despite sender errors")
	}

	// Clear the error and verify events can now be sent
	sender.setError(nil)

	// Create another pod
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "recovery-pod",
			Namespace: "default",
			UID:       types.UID("recovery-pod-uid"),
		},
	}
	_, err = clientset.CoreV1().Pods("default").Create(ctx, pod2, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Should have events now
	events := sender.getEvents()
	if len(events) == 0 {
		t.Error("expected events after error cleared")
	}

	mgr.Stop()
	<-errCh
}

// TestIntegration_GracefulShutdownWithPendingEvents tests that pending events are drained on shutdown.
func TestIntegration_GracefulShutdownWithPendingEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientset := fake.NewSimpleClientset()

	// Use a slow sender that tracks how many events were eventually processed
	var processedCount int32
	slowSender := &slowMockEventSender{
		delay:          50 * time.Millisecond,
		processedCount: &processedCount,
	}

	log := ctrl.Log.WithName("test-graceful-shutdown")

	cfg := ManagerConfig{
		ClusterUID:      "shutdown-cluster",
		EventSender:     slowSender,
		EventBufferSize: 100,
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Create several pods quickly
	for i := 0; i < 10; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shutdown-pod-" + string(rune('0'+i)),
				Namespace: "default",
				UID:       types.UID("shutdown-uid-" + string(rune('0'+i))),
			},
		}
		_, _ = clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	}

	// Stop manager immediately - should drain remaining events
	mgr.Stop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("timeout waiting for Start() to return")
	}

	// Some events should have been processed during drain
	final := atomic.LoadInt32(&processedCount)
	if final == 0 {
		t.Error("expected some events to be processed during drain")
	}
	t.Logf("processed %d events during graceful shutdown", final)
}

// TestIntegration_GetCurrentStateAfterSync tests GetCurrentState returns correct data after sync.
func TestIntegration_GetCurrentStateAfterSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pre-populate with resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "state-ns",
			UID:  types.UID("state-ns-uid"),
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "state-pod",
			Namespace: "default",
			UID:       types.UID("state-pod-uid"),
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "state-svc",
			Namespace: "default",
			UID:       types.UID("state-svc-uid"),
		},
	}

	clientset := fake.NewSimpleClientset(ns, pod, svc)
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test-get-state")

	cfg := ManagerConfig{
		ClusterUID:  "state-cluster",
		EventSender: sender,
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Get current state
	state, err := mgr.GetCurrentState()
	if err != nil {
		t.Fatalf("GetCurrentState() error: %v", err)
	}

	if state.ClusterUID != "state-cluster" {
		t.Errorf("ClusterUID = %q, want %q", state.ClusterUID, "state-cluster")
	}

	// Should have our resources
	foundNS := false
	for _, n := range state.Namespaces {
		if n.Name == "state-ns" {
			foundNS = true
			break
		}
	}
	if !foundNS {
		t.Error("expected to find state-ns in current state")
	}

	foundPod := false
	for _, p := range state.Pods {
		if p.Name == "state-pod" {
			foundPod = true
			break
		}
	}
	if !foundPod {
		t.Error("expected to find state-pod in current state")
	}

	foundSvc := false
	for _, s := range state.Services {
		if s.Name == "state-svc" {
			foundSvc = true
			break
		}
	}
	if !foundSvc {
		t.Error("expected to find state-svc in current state")
	}

	mgr.Stop()
	<-errCh
}

// TestIntegration_MultipleIngestersNoDeadlock tests that multiple ingesters don't deadlock.
func TestIntegration_MultipleIngestersNoDeadlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientset := fake.NewSimpleClientset()
	sender := newMockEventSender()
	log := ctrl.Log.WithName("test-no-deadlock")

	cfg := ManagerConfig{
		ClusterUID:      "deadlock-cluster",
		EventSender:     sender,
		EventBufferSize: 10, // Small buffer to stress test
	}

	mgr := NewManager(clientset, cfg, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Create resources of all types rapidly
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		idx := i

		go func() {
			defer wg.Done()
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dl-ns-" + string(rune('a'+idx%26)),
					UID:  types.UID("dl-ns-uid-" + string(rune('0'+idx))),
				},
			}
			clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		}()

		go func() {
			defer wg.Done()
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dl-pod-" + string(rune('a'+idx%26)),
					Namespace: "default",
					UID:       types.UID("dl-pod-uid-" + string(rune('0'+idx))),
				},
			}
			clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
		}()

		go func() {
			defer wg.Done()
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dl-svc-" + string(rune('a'+idx%26)),
					Namespace: "default",
					UID:       types.UID("dl-svc-uid-" + string(rune('0'+idx))),
				},
			}
			clientset.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock detected - operations did not complete")
	}

	mgr.Stop()
	<-errCh
}

// slowMockEventSender is a mock that introduces delay in event processing.
type slowMockEventSender struct {
	delay          time.Duration
	processedCount *int32
}

func (s *slowMockEventSender) SendEvent(ctx context.Context, payload *transport.EventPayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.delay):
		atomic.AddInt32(s.processedCount, 1)
		return nil
	}
}
