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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSecretIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	immutable := true
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "1",
			Labels:          map[string]string{"app": "test"},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("SENSITIVE-CERTIFICATE-DATA-MUST-NEVER-BE-SENT"),
			"tls.key": []byte("SENSITIVE-PRIVATE-KEY-MUST-NEVER-BE-SENT"),
		},
		Immutable: &immutable,
	}

	ingester.onAdd(secret)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %s, want ADDED", event.Type)
		}
		if event.Kind != transport.ResourceTypeSecret {
			t.Errorf("Kind = %s, want Secret", event.Kind)
		}
		if event.Name != "test-secret" {
			t.Errorf("Name = %s, want test-secret", event.Name)
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %s, want default", event.Namespace)
		}

		// Verify the object is SecretInfo
		secretInfo, ok := event.Object.(transport.SecretInfo)
		if !ok {
			t.Fatal("Object is not SecretInfo")
		}

		// SECURITY: Verify only keys are included, not values
		if len(secretInfo.DataKeys) != 2 {
			t.Errorf("DataKeys count = %d, want 2", len(secretInfo.DataKeys))
		}
		if secretInfo.Type != "kubernetes.io/tls" {
			t.Errorf("Type = %s, want kubernetes.io/tls", secretInfo.Type)
		}
		if !secretInfo.Immutable {
			t.Error("Immutable should be true")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSecretIngester_OnUpdate(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "1",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"key1": []byte("value1")},
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "2",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
	}

	// First add the old version to track it
	ingester.onAdd(oldSecret)
	<-eventChan // Drain the add event

	// Now update
	ingester.onUpdate(oldSecret, newSecret)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %s, want UPDATED", event.Type)
		}
		if event.Kind != transport.ResourceTypeSecret {
			t.Errorf("Kind = %s, want Secret", event.Kind)
		}

		secretInfo, ok := event.Object.(transport.SecretInfo)
		if !ok {
			t.Fatal("Object is not SecretInfo")
		}
		if len(secretInfo.DataKeys) != 2 {
			t.Errorf("DataKeys count = %d, want 2", len(secretInfo.DataKeys))
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSecretIngester_OnDelete(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "1",
		},
	}

	ingester.onDelete(secret)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %s, want DELETED", event.Type)
		}
		if event.Kind != transport.ResourceTypeSecret {
			t.Errorf("Kind = %s, want Secret", event.Kind)
		}
		if event.Name != "test-secret" {
			t.Errorf("Name = %s, want test-secret", event.Name)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete events")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSecretIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 0) // Zero buffer - always full
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "1",
		},
	}

	// This should not block - event should be dropped
	done := make(chan bool)
	go func() {
		ingester.onAdd(secret)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(time.Second):
		t.Fatal("onAdd blocked when channel was full")
	}
}

func TestSecretIngester_SkipSameResourceVersion(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			UID:             "secret-uid-123",
			ResourceVersion: "1",
		},
	}

	// Add first
	ingester.onAdd(secret)
	<-eventChan

	// Update with same version should be skipped
	ingester.onUpdate(secret, secret)

	select {
	case <-eventChan:
		t.Fatal("received event for duplicate update")
	case <-time.After(100 * time.Millisecond):
		// Success - no event for duplicate
	}
}

// SECURITY TEST: This is the most critical test - verify that secret data values
// are NEVER included in the transmitted data.
func TestSecretIngester_SecurityNoDataValues(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := testr.New(t)

	ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Create a secret with highly sensitive data
	sensitivePassword := "SUPER-SECRET-DATABASE-PASSWORD-12345"
	sensitiveKey := "PRIVATE-KEY-THAT-MUST-NEVER-LEAK"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "sensitive-secret",
			Namespace:       "production",
			UID:             "secret-uid-sensitive",
			ResourceVersion: "1",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password":    []byte(sensitivePassword),
			"private-key": []byte(sensitiveKey),
			"api-token":   []byte("ANOTHER-SENSITIVE-TOKEN"),
		},
	}

	ingester.onAdd(secret)

	select {
	case event := <-eventChan:
		secretInfo, ok := event.Object.(transport.SecretInfo)
		if !ok {
			t.Fatal("Object is not SecretInfo")
		}

		// Verify that we have the expected keys
		if len(secretInfo.DataKeys) != 3 {
			t.Errorf("DataKeys count = %d, want 3", len(secretInfo.DataKeys))
		}

		// SECURITY: Verify that data keys contain only key names, not values
		foundPassword := false
		foundPrivateKey := false
		foundApiToken := false
		for _, key := range secretInfo.DataKeys {
			if key == "password" {
				foundPassword = true
			}
			if key == "private-key" {
				foundPrivateKey = true
			}
			if key == "api-token" {
				foundApiToken = true
			}

			// CRITICAL SECURITY CHECK: Ensure NO sensitive values are present
			if key == sensitivePassword {
				t.Fatalf("SECURITY VIOLATION: Sensitive password value found in DataKeys!")
			}
			if key == sensitiveKey {
				t.Fatalf("SECURITY VIOLATION: Sensitive private key value found in DataKeys!")
			}
		}

		if !foundPassword {
			t.Error("Expected 'password' key in DataKeys")
		}
		if !foundPrivateKey {
			t.Error("Expected 'private-key' key in DataKeys")
		}
		if !foundApiToken {
			t.Error("Expected 'api-token' key in DataKeys")
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// Test various secret types are handled correctly
func TestSecretIngester_SecretTypes(t *testing.T) {
	testCases := []struct {
		name       string
		secretType corev1.SecretType
		expected   string
	}{
		{
			name:       "Opaque",
			secretType: corev1.SecretTypeOpaque,
			expected:   "Opaque",
		},
		{
			name:       "TLS",
			secretType: corev1.SecretTypeTLS,
			expected:   "kubernetes.io/tls",
		},
		{
			name:       "DockerConfigJson",
			secretType: corev1.SecretTypeDockerConfigJson,
			expected:   "kubernetes.io/dockerconfigjson",
		},
		{
			name:       "ServiceAccountToken",
			secretType: corev1.SecretTypeServiceAccountToken,
			expected:   "kubernetes.io/service-account-token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			factory := informers.NewSharedInformerFactory(clientset, 0)
			eventChan := make(chan ResourceEvent, 10)
			log := testr.New(t)

			ingester := NewSecretIngester(factory, IngesterConfig{EventChan: eventChan}, log)
			ingester.RegisterHandlers()

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-secret",
					Namespace:       "default",
					UID:             types.UID("secret-uid-" + tc.name),
					ResourceVersion: "1",
				},
				Type: tc.secretType,
			}

			ingester.onAdd(secret)

			select {
			case event := <-eventChan:
				secretInfo, ok := event.Object.(transport.SecretInfo)
				if !ok {
					t.Fatal("Object is not SecretInfo")
				}
				if secretInfo.Type != tc.expected {
					t.Errorf("Type = %s, want %s", secretInfo.Type, tc.expected)
				}

			case <-time.After(time.Second):
				t.Fatal("timeout waiting for event")
			}
		})
	}
}
