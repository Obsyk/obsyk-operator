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
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

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

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

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

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

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
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

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
	if !result.IsZero() {
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

// TestReconciler_CheckPlatformHealth tests the platform health check.
func TestReconciler_CheckPlatformHealth(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	// With no agents, should be healthy
	err := reconciler.CheckPlatformHealth()
	if err != nil {
		t.Errorf("expected healthy when no agents, got: %v", err)
	}
}

// TestReconciler_CheckReady tests the readiness check.
func TestReconciler_CheckReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	// With no agents, should be ready
	err := reconciler.CheckReady()
	if err != nil {
		t.Errorf("expected ready when no agents, got: %v", err)
	}
}

// TestReconciler_GetClusterUID tests cluster UID detection.
func TestReconciler_GetClusterUID(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-12345",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	uid, err := reconciler.getClusterUID(context.Background())
	if err != nil {
		t.Fatalf("getClusterUID failed: %v", err)
	}

	if uid != "test-cluster-uid-12345" {
		t.Errorf("expected uid 'test-cluster-uid-12345', got '%s'", uid)
	}
}

// TestReconciler_GetClusterUIDNotFound tests cluster UID detection when kube-system is missing.
func TestReconciler_GetClusterUIDNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	_, err := reconciler.getClusterUID(context.Background())
	if err == nil {
		t.Error("expected error when kube-system namespace not found")
	}
}

// TestReconciler_GetResourceCounts tests resource counting.
func TestReconciler_GetResourceCounts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	// Create some resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns, pod, svc).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	counts, err := reconciler.getResourceCounts(context.Background())
	if err != nil {
		t.Fatalf("getResourceCounts failed: %v", err)
	}

	if counts.Namespaces != 1 {
		t.Errorf("expected 1 namespace, got %d", counts.Namespaces)
	}
	if counts.Pods != 1 {
		t.Errorf("expected 1 pod, got %d", counts.Pods)
	}
	if counts.Services != 1 {
		t.Errorf("expected 1 service, got %d", counts.Services)
	}
}

// TestReconciler_SetCondition tests setting conditions on agent status.
func TestReconciler_SetCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	agent := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 1,
		},
	}

	// Set a condition
	reconciler.setCondition(agent, obsykv1.ConditionTypeAvailable, metav1.ConditionTrue, "TestReason", "Test message")

	// Verify condition was set
	found := false
	for _, c := range agent.Status.Conditions {
		if c.Type == string(obsykv1.ConditionTypeAvailable) {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected status True, got %s", c.Status)
			}
			if c.Reason != "TestReason" {
				t.Errorf("expected reason TestReason, got %s", c.Reason)
			}
			if c.Message != "Test message" {
				t.Errorf("expected message 'Test message', got %s", c.Message)
			}
		}
	}
	if !found {
		t.Error("condition was not set")
	}
}

// TestReconciler_GetCredentials tests credential retrieval.
func TestReconciler_GetCredentials(t *testing.T) {
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
			CredentialsSecretRef: obsykv1.SecretReference{
				Name: "test-secret",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	creds, err := reconciler.getCredentials(context.Background(), agent)
	if err != nil {
		t.Fatalf("getCredentials failed: %v", err)
	}

	if creds.ClientID != "test-client-id" {
		t.Errorf("expected client_id 'test-client-id', got '%s'", creds.ClientID)
	}
}

// TestReconciler_GetCredentialsNotFound tests credential retrieval when secret is missing.
func TestReconciler_GetCredentialsNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	agent := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: obsykv1.ObsykAgentSpec{
			CredentialsSecretRef: obsykv1.SecretReference{
				Name: "missing-secret",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	_, err := reconciler.getCredentials(context.Background(), agent)
	if err == nil {
		t.Error("expected error when secret not found")
	}
}

// TestReconciler_FindAgentsForResource tests the agent finder function.
func TestReconciler_FindAgentsForResource(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	agent1 := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-1",
			Namespace: "default",
		},
	}
	agent2 := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-2",
			Namespace: "other",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(agent1, agent2).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	// Create a test pod to trigger the lookup
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	requests := reconciler.findAgentsForResource(context.Background(), pod)

	if len(requests) != 2 {
		t.Errorf("expected 2 reconcile requests, got %d", len(requests))
	}
}

// TestReconciler_CheckPlatformHealthWithUnhealthyAgent tests health check with unhealthy agents.
func TestReconciler_CheckPlatformHealthWithUnhealthyAgent(t *testing.T) {
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

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	// Create an agent client (which will be unhealthy initially since it has never connected)
	_, err := reconciler.getOrCreateAgentClient(context.Background(), agent)
	if err != nil {
		t.Fatalf("Failed to create agent client: %v", err)
	}

	// Platform should be unhealthy since no successful connection has been made
	err = reconciler.CheckPlatformHealth()
	if err == nil {
		t.Error("expected error for unhealthy agent")
	}
}

// TestReconciler_CheckReadyWithNoSyncedAgent tests readiness check with no synced agents.
func TestReconciler_CheckReadyWithNoSyncedAgent(t *testing.T) {
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

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	// Create an agent client (which will not have synced initially)
	_, err := reconciler.getOrCreateAgentClient(context.Background(), agent)
	if err != nil {
		t.Fatalf("Failed to create agent client: %v", err)
	}

	// Should not be ready since no agent has completed initial sync
	err = reconciler.CheckReady()
	if err == nil {
		t.Error("expected error for agent without initial sync")
	}
}

// TestReconciler_ReconcileAgentExists tests reconciliation when agent exists.
func TestReconciler_ReconcileAgentExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	testPrivateKey := generateTestPrivateKey(t)

	// Create kube-system namespace for cluster UID
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "cluster-uid-12345",
		},
	}

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
		WithObjects(kubeSystemNS, secret, agent).
		WithStatusSubresource(agent).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-agent",
			Namespace: "default",
		},
	}

	// With nil clientset, the ingestion manager won't start, so we'll get a requeue
	result, err := reconciler.Reconcile(context.Background(), req)
	// We expect a requeue to wait for ingestion manager to start
	if result.RequeueAfter == 0 && err == nil {
		t.Error("expected requeue while waiting for ingestion manager")
	}
}

// TestReconciler_ReconcileMissingSecret tests reconciliation when secret is missing.
func TestReconciler_ReconcileMissingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	agent := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: obsykv1.ObsykAgentSpec{
			ClusterName: "test-cluster",
			PlatformURL: "https://api.example.com",
			CredentialsSecretRef: obsykv1.SecretReference{
				Name: "missing-secret",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(agent).
		WithStatusSubresource(agent).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-agent",
			Namespace: "default",
		},
	}

	// Should fail at credential retrieval
	result, _ := reconciler.Reconcile(context.Background(), req)
	// We expect a requeue due to credential error
	if result.RequeueAfter == 0 {
		t.Error("expected requeue due to missing secret")
	}
}

// TestReconciler_GetCredentialsInvalidKey tests credential retrieval with invalid private key.
func TestReconciler_GetCredentialsInvalidKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"client_id":   []byte("test-client-id"),
			"private_key": []byte("not-a-valid-pem-key"),
		},
	}

	agent := &obsykv1.ObsykAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: obsykv1.ObsykAgentSpec{
			CredentialsSecretRef: obsykv1.SecretReference{
				Name: "bad-secret",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	_, err := reconciler.getCredentials(context.Background(), agent)
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}

// TestReconciler_FindAgentsForResourceEmpty tests finder when no agents exist.
func TestReconciler_FindAgentsForResourceEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = obsykv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewObsykAgentReconciler(fakeClient, fakeClient, scheme, nil)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	requests := reconciler.findAgentsForResource(context.Background(), pod)

	if len(requests) != 0 {
		t.Errorf("expected 0 reconcile requests, got %d", len(requests))
	}
}
