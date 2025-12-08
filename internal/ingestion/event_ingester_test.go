// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestEventIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	now := metav1.Now()
	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
			UID:       "event-uid-123",
		},
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "0/3 nodes are available: insufficient memory.",
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-456",
		},
		Source: corev1.EventSource{
			Component: "scheduler",
			Host:      "node-1",
		},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}

	_, err := clientset.CoreV1().Events("default").Create(context.Background(), k8sEvent, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeEvent {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeEvent)
		}
		if event.Name != "test-event" {
			t.Errorf("Name = %v, want %v", event.Name, "test-event")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		eventInfo, ok := event.Object.(transport.EventInfo)
		if !ok {
			t.Errorf("Object is not EventInfo: %T", event.Object)
		} else {
			if eventInfo.Type != "Warning" {
				t.Errorf("EventInfo.Type = %s, want Warning", eventInfo.Type)
			}
			if eventInfo.Reason != "FailedScheduling" {
				t.Errorf("EventInfo.Reason = %s, want FailedScheduling", eventInfo.Reason)
			}
			if eventInfo.InvolvedObject.Kind != "Pod" {
				t.Errorf("InvolvedObject.Kind = %s, want Pod", eventInfo.InvolvedObject.Kind)
			}
			if eventInfo.InvolvedObject.Name != "test-pod" {
				t.Errorf("InvolvedObject.Name = %s, want test-pod", eventInfo.InvolvedObject.Name)
			}
			if eventInfo.Source.Component != "scheduler" {
				t.Errorf("Source.Component = %s, want scheduler", eventInfo.Source.Component)
			}
			if eventInfo.Count != 1 {
				t.Errorf("Count = %d, want 1", eventInfo.Count)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestEventIngester_OnUpdate(t *testing.T) {
	now := metav1.Now()
	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-event",
			Namespace:       "default",
			UID:             "event-uid-123",
			ResourceVersion: "1",
		},
		Type:           "Warning",
		Reason:         "FailedScheduling",
		Message:        "0/3 nodes are available.",
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}

	clientset := fake.NewSimpleClientset(k8sEvent)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Update the event (simulating event count increase)
	updatedEvent := k8sEvent.DeepCopy()
	updatedEvent.ResourceVersion = "2"
	updatedEvent.Count = 5
	updatedEvent.LastTimestamp = metav1.Now()

	_, err := clientset.CoreV1().Events("default").Update(context.Background(), updatedEvent, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update event: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeEvent {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeEvent)
		}
		eventInfo, ok := event.Object.(transport.EventInfo)
		if !ok {
			t.Errorf("Object is not EventInfo: %T", event.Object)
		} else {
			if eventInfo.Count != 5 {
				t.Errorf("Count = %d, want 5", eventInfo.Count)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestEventIngester_OnDelete(t *testing.T) {
	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
			UID:       "event-uid-123",
		},
		Type:   "Normal",
		Reason: "Scheduled",
	}

	clientset := fake.NewSimpleClientset(k8sEvent)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Delete the event
	err := clientset.CoreV1().Events("default").Delete(context.Background(), "test-event", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete event: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeEvent {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeEvent)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestEventIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
			UID:       "event-uid-123",
		},
		Type:   "Normal",
		Reason: "Created",
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.CoreV1().Events("default").Create(context.Background(), k8sEvent, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestEventIngester_SkipSameResourceVersion(t *testing.T) {
	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-event",
			Namespace:       "default",
			UID:             "event-uid-123",
			ResourceVersion: "1",
		},
		Type:   "Normal",
		Reason: "Scheduled",
	}

	clientset := fake.NewSimpleClientset(k8sEvent)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Manually call onUpdate with same resource version - should be skipped
	ingester.onUpdate(k8sEvent, k8sEvent)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestEventIngester_WarningEvent(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewEventIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	now := metav1.Now()
	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oom-event",
			Namespace: "production",
			UID:       "event-uid-oom",
		},
		Type:    "Warning",
		Reason:  "OOMKilled",
		Message: "Container was killed due to OOM.",
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "my-app-abc123",
			Namespace: "production",
			UID:       "pod-uid-abc",
		},
		Source: corev1.EventSource{
			Component: "kubelet",
			Host:      "worker-node-2",
		},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          3,
	}

	_, err := clientset.CoreV1().Events("production").Create(context.Background(), k8sEvent, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	select {
	case event := <-eventChan:
		eventInfo, ok := event.Object.(transport.EventInfo)
		if !ok {
			t.Fatalf("Object is not EventInfo: %T", event.Object)
		}

		if eventInfo.Type != "Warning" {
			t.Errorf("Type = %s, want Warning", eventInfo.Type)
		}
		if eventInfo.Reason != "OOMKilled" {
			t.Errorf("Reason = %s, want OOMKilled", eventInfo.Reason)
		}
		if eventInfo.InvolvedObject.Kind != "Pod" {
			t.Errorf("InvolvedObject.Kind = %s, want Pod", eventInfo.InvolvedObject.Kind)
		}
		if eventInfo.Source.Component != "kubelet" {
			t.Errorf("Source.Component = %s, want kubelet", eventInfo.Source.Component)
		}
		if eventInfo.Source.Host != "worker-node-2" {
			t.Errorf("Source.Host = %s, want worker-node-2", eventInfo.Source.Host)
		}
		if eventInfo.Count != 3 {
			t.Errorf("Count = %d, want 3", eventInfo.Count)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}
