// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestConfigMapIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	immutable := true
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "1",
			Labels:          map[string]string{"app": "test"},
		},
		Data: map[string]string{
			"config.yaml":   "sensitive-value-should-not-be-sent",
			"settings.json": "another-sensitive-value",
		},
		BinaryData: map[string][]byte{
			"binary.dat": []byte("binary-data-should-not-be-sent"),
		},
		Immutable: &immutable,
	}

	ingester.onAdd(cm)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %s, want ADDED", event.Type)
		}
		if event.Kind != transport.ResourceTypeConfigMap {
			t.Errorf("Kind = %s, want ConfigMap", event.Kind)
		}
		if event.Name != "test-configmap" {
			t.Errorf("Name = %s, want test-configmap", event.Name)
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %s, want default", event.Namespace)
		}

		// Verify the object is ConfigMapInfo
		cmInfo, ok := event.Object.(transport.ConfigMapInfo)
		if !ok {
			t.Fatal("Object is not ConfigMapInfo")
		}

		// SECURITY: Verify only keys are included, not values
		if len(cmInfo.DataKeys) != 2 {
			t.Errorf("DataKeys count = %d, want 2", len(cmInfo.DataKeys))
		}
		if len(cmInfo.BinaryKeys) != 1 {
			t.Errorf("BinaryKeys count = %d, want 1", len(cmInfo.BinaryKeys))
		}
		if !cmInfo.Immutable {
			t.Error("Immutable should be true")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestConfigMapIngester_OnUpdate(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	oldCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "1",
		},
		Data: map[string]string{"key1": "value1"},
	}

	newCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "2",
		},
		Data: map[string]string{"key1": "value1", "key2": "value2"},
	}

	// First add the old version to track it
	ingester.onAdd(oldCM)
	<-eventChan // Drain the add event

	// Now update
	ingester.onUpdate(oldCM, newCM)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %s, want UPDATED", event.Type)
		}
		if event.Kind != transport.ResourceTypeConfigMap {
			t.Errorf("Kind = %s, want ConfigMap", event.Kind)
		}

		cmInfo, ok := event.Object.(transport.ConfigMapInfo)
		if !ok {
			t.Fatal("Object is not ConfigMapInfo")
		}
		if len(cmInfo.DataKeys) != 2 {
			t.Errorf("DataKeys count = %d, want 2", len(cmInfo.DataKeys))
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestConfigMapIngester_OnDelete(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "1",
		},
	}

	ingester.onDelete(cm)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %s, want DELETED", event.Type)
		}
		if event.Kind != transport.ResourceTypeConfigMap {
			t.Errorf("Kind = %s, want ConfigMap", event.Kind)
		}
		if event.Name != "test-configmap" {
			t.Errorf("Name = %s, want test-configmap", event.Name)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete events")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestConfigMapIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent) // Zero buffer - always full
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "1",
		},
	}

	// This should not block - event should be dropped
	done := make(chan bool)
	go func() {
		ingester.onAdd(cm)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(time.Second):
		t.Fatal("onAdd blocked when channel was full")
	}
}

func TestConfigMapIngester_SkipSameResourceVersion(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-configmap",
			Namespace:       "default",
			UID:             "cm-uid-123",
			ResourceVersion: "1",
		},
	}

	// Add first
	ingester.onAdd(cm)
	<-eventChan

	// Update with same version should be skipped
	ingester.onUpdate(cm, cm)

	select {
	case <-eventChan:
		t.Fatal("received event for duplicate update")
	case <-time.After(100 * time.Millisecond):
		// Success - no event for duplicate
	}
}

// SECURITY TEST: Verify that data values are never included
func TestConfigMapIngester_SecurityNoDataValues(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewConfigMapIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	sensitiveValue := "THIS-SENSITIVE-VALUE-MUST-NOT-BE-TRANSMITTED"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "sensitive-configmap",
			Namespace:       "default",
			UID:             "cm-uid-sensitive",
			ResourceVersion: "1",
		},
		Data: map[string]string{
			"database-password": sensitiveValue,
			"api-key":           "another-sensitive-value",
		},
	}

	ingester.onAdd(cm)

	select {
	case event := <-eventChan:
		cmInfo, ok := event.Object.(transport.ConfigMapInfo)
		if !ok {
			t.Fatal("Object is not ConfigMapInfo")
		}

		// Verify keys are present
		foundPasswordKey := false
		foundApiKey := false
		for _, key := range cmInfo.DataKeys {
			if key == "database-password" {
				foundPasswordKey = true
			}
			if key == "api-key" {
				foundApiKey = true
			}
			// SECURITY: Ensure value is never the key
			if key == sensitiveValue {
				t.Fatalf("SECURITY VIOLATION: Sensitive value found in DataKeys")
			}
		}

		if !foundPasswordKey {
			t.Error("Expected database-password key in DataKeys")
		}
		if !foundApiKey {
			t.Error("Expected api-key key in DataKeys")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
