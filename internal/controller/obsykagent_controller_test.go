// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	obsykv1 "github.com/obsyk/obsyk-operator/api/v1"
)

// generateTestPrivateKey generates a test ECDSA P-256 private key in PEM format
func generateTestPrivateKey(t *testing.T) string {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	return string(pemBlock)
}

func TestNewObsykAgentReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, scheme)

	if reconciler == nil {
		t.Fatal("expected non-nil reconciler")
	}
	if reconciler.agentClients == nil {
		t.Error("expected agentClients map to be initialized")
	}
	if reconciler.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
}

// TestReconciler_ConcurrentAgentClientAccess tests that concurrent access to
// agentClients map is safe.
func TestReconciler_ConcurrentAgentClientAccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	testPrivateKey := generateTestPrivateKey(t)

	// Create test secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"client_id":   []byte("test-client-id"),
			"private_key": []byte(testPrivateKey),
		},
	}

	// Create multiple ObsykAgent resources
	agents := make([]*obsykv1.ObsykAgent, 10)
	for i := 0; i < 10; i++ {
		agents[i] = &obsykv1.ObsykAgent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-" + string(rune('0'+i)),
				Namespace: "default",
			},
			Spec: obsykv1.ObsykAgentSpec{
				ClusterName: "test-cluster",
				PlatformURL: "https://api.example.com",
				CredentialsSecretRef: obsykv1.SecretReference{
					Name: "test-secret",
				},
			},
		}
	}

	clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret)
	for _, agent := range agents {
		clientBuilder = clientBuilder.WithObjects(agent)
	}
	fakeClient := clientBuilder.Build()

	reconciler := NewObsykAgentReconciler(fakeClient, scheme)

	// Concurrently call getOrCreateAgentClient for different agents
	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agent := agents[idx%len(agents)]
			ctx := context.Background()

			// This should not race
			_, err := reconciler.getOrCreateAgentClient(ctx, agent)
			if err != nil {
				t.Errorf("getOrCreateAgentClient failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify clients were created
	reconciler.agentClientsMu.RLock()
	clientCount := len(reconciler.agentClients)
	reconciler.agentClientsMu.RUnlock()

	if clientCount == 0 {
		t.Error("expected at least one agent client to be created")
	}
	t.Logf("created %d agent clients", clientCount)
}

// TestReconciler_ConcurrentDeleteAndCreate tests concurrent deletion and creation
// of agent clients.
func TestReconciler_ConcurrentDeleteAndCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	testPrivateKey := generateTestPrivateKey(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"client_id":   []byte("test-client-id"),
			"private_key": []byte(testPrivateKey),
		},
	}

	agent := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: obsykv1.ObsykAgentSpec{
			ClusterName: "test-cluster",
			PlatformURL: "https://api.example.com",
			CredentialsSecretRef: obsykv1.SecretReference{
				Name: "test-secret",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, agent).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, scheme)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Goroutines that create/update clients
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, _ = reconciler.getOrCreateAgentClient(context.Background(), agent)
				}
			}
		}()
	}

	// Goroutines that delete clients
	key := "default/test-agent"
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					reconciler.agentClientsMu.Lock()
					delete(reconciler.agentClients, key)
					reconciler.agentClientsMu.Unlock()
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	// If we get here without race detector complaints, test passes
}

// TestReconciler_ReconcileNotFound tests that Reconcile handles deleted resources
// safely.
func TestReconciler_ReconcileNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, scheme)

	// Pre-populate with a client that should be deleted
	reconciler.agentClientsMu.Lock()
	reconciler.agentClients["default/deleted-agent"] = &agentClient{}
	reconciler.agentClientsMu.Unlock()

	// Reconcile for a non-existent agent
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "deleted-agent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile returned error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue for deleted resource")
	}

	// Verify client was cleaned up
	reconciler.agentClientsMu.RLock()
	_, exists := reconciler.agentClients["default/deleted-agent"]
	reconciler.agentClientsMu.RUnlock()

	if exists {
		t.Error("expected agent client to be deleted")
	}
}
