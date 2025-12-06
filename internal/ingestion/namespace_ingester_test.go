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

func TestNamespaceIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNamespaceIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			UID:  "ns-uid-123",
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeNamespace {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNamespace)
		}
		if event.Name != "test-ns" {
			t.Errorf("Name = %v, want %v", event.Name, "test-ns")
		}
		// Namespace is cluster-scoped, so Namespace field should be empty
		if event.Namespace != "" {
			t.Errorf("Namespace = %v, want empty string", event.Namespace)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestNamespaceIngester_OnUpdate(t *testing.T) {
	// Create initial namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ns",
			UID:             "ns-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(ns)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNamespaceIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
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

	// Update the namespace
	updatedNS := ns.DeepCopy()
	updatedNS.ResourceVersion = "2"
	updatedNS.Labels = map[string]string{"updated": "true"}

	_, err := clientset.CoreV1().Namespaces().Update(context.TODO(), updatedNS, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update namespace: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeNamespace {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNamespace)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestNamespaceIngester_OnDelete(t *testing.T) {
	// Create initial namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			UID:  "ns-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(ns)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNamespaceIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
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

	// Delete the namespace
	err := clientset.CoreV1().Namespaces().Delete(context.TODO(), "test-ns", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete namespace: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeNamespace {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNamespace)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestNamespaceIngester_ClusterScoped(t *testing.T) {
	// Verify namespace events have empty Namespace field (cluster-scoped)
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNamespaceIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			UID:  "ns-uid-prod",
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Check event
	select {
	case event := <-eventChan:
		if event.Namespace != "" {
			t.Errorf("Namespace field should be empty for cluster-scoped resource, got: %q", event.Namespace)
		}
		if event.Name != "production" {
			t.Errorf("Name = %q, want %q", event.Name, "production")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for event")
	}
}
