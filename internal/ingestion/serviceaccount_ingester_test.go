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

func TestServiceAccountIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewServiceAccountIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	automount := true
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "default",
			UID:       "sa-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Secrets: []corev1.ObjectReference{
			{Name: "sa-token-secret"},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "docker-registry"},
		},
		AutomountServiceAccountToken: &automount,
	}

	_, err := clientset.CoreV1().ServiceAccounts("default").Create(context.Background(), sa, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create serviceaccount: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeServiceAccount {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeServiceAccount)
		}
		if event.Name != "test-sa" {
			t.Errorf("Name = %v, want %v", event.Name, "test-sa")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		saInfo, ok := event.Object.(transport.ServiceAccountInfo)
		if !ok {
			t.Errorf("Object is not ServiceAccountInfo: %T", event.Object)
		} else {
			if len(saInfo.Secrets) != 1 {
				t.Errorf("Secrets count = %d, want 1", len(saInfo.Secrets))
			}
			if len(saInfo.ImagePullSecrets) != 1 {
				t.Errorf("ImagePullSecrets count = %d, want 1", len(saInfo.ImagePullSecrets))
			}
			if saInfo.AutomountServiceAccountToken == nil || !*saInfo.AutomountServiceAccountToken {
				t.Error("AutomountServiceAccountToken should be true")
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestServiceAccountIngester_OnUpdate(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sa",
			Namespace:       "default",
			UID:             "sa-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(sa)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewServiceAccountIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the serviceaccount
	updatedSA := sa.DeepCopy()
	updatedSA.ResourceVersion = "2"
	updatedSA.Labels = map[string]string{"updated": "true"}

	_, err := clientset.CoreV1().ServiceAccounts("default").Update(context.Background(), updatedSA, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update serviceaccount: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeServiceAccount {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeServiceAccount)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestServiceAccountIngester_OnDelete(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "default",
			UID:       "sa-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(sa)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewServiceAccountIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the serviceaccount
	err := clientset.CoreV1().ServiceAccounts("default").Delete(context.Background(), "test-sa", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete serviceaccount: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeServiceAccount {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeServiceAccount)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestServiceAccountIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewServiceAccountIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "default",
			UID:       "sa-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.CoreV1().ServiceAccounts("default").Create(context.Background(), sa, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestServiceAccountIngester_SkipSameResourceVersion(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sa",
			Namespace:       "default",
			UID:             "sa-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(sa)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewServiceAccountIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(sa, sa)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
