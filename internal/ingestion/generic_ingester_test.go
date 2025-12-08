// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// TestGenericIngester_NamespacedResource tests the generic ingester with a namespaced resource (Pod).
func TestGenericIngester_NamespacedResource(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	// Create a Pod ingester using the generic implementation
	ingester := NewGenericIngester(PodConfig(factory), cfg, log)

	if err := ingester.RegisterHandlers(); err != nil {
		t.Fatalf("failed to register handlers: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Test Add event - use clientset to trigger informer
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-123",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx:latest"},
			},
		},
	}

	_, err := clientset.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("expected Added event, got %s", event.Type)
		}
		if event.Kind != transport.ResourceTypePod {
			t.Errorf("expected Pod kind, got %s", event.Kind)
		}
		if event.Name != "test-pod" {
			t.Errorf("expected name 'test-pod', got '%s'", event.Name)
		}
		if event.Namespace != "default" {
			t.Errorf("expected namespace 'default', got '%s'", event.Namespace)
		}
		if event.UID != "pod-uid-123" {
			t.Errorf("expected UID 'pod-uid-123', got '%s'", event.UID)
		}
		if event.Object == nil {
			t.Error("expected Object to be non-nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for add event")
	}

	// Test Update event
	// Get the pod from the API to get the current ResourceVersion
	updatedPod, err := clientset.CoreV1().Pods("default").Get(context.TODO(), "test-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}
	updatedPod.Labels = map[string]string{"env": "test"}
	// Increment ResourceVersion to ensure the update handler processes it
	updatedPod.ResourceVersion = "2"
	_, err = clientset.CoreV1().Pods("default").Update(context.TODO(), updatedPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update pod: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("expected Modified event, got %s", event.Type)
		}
		if event.Kind != transport.ResourceTypePod {
			t.Errorf("expected Pod kind, got %s", event.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for update event")
	}

	// Test Delete event
	err = clientset.CoreV1().Pods("default").Delete(context.TODO(), "test-pod", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete pod: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("expected Deleted event, got %s", event.Type)
		}
		if event.Object != nil {
			t.Error("expected Object to be nil for delete events")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for delete event")
	}
}

// TestGenericIngester_ClusterScopedResource tests the generic ingester with a cluster-scoped resource (Namespace).
func TestGenericIngester_ClusterScopedResource(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	// Create a Namespace ingester using the generic implementation
	ingester := NewGenericIngester(NamespaceConfig(factory), cfg, log)

	if err := ingester.RegisterHandlers(); err != nil {
		t.Fatalf("failed to register handlers: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Test Add event for cluster-scoped resource
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			UID:  "ns-uid-456",
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("expected Added event, got %s", event.Type)
		}
		if event.Kind != transport.ResourceTypeNamespace {
			t.Errorf("expected Namespace kind, got %s", event.Kind)
		}
		if event.Name != "test-namespace" {
			t.Errorf("expected name 'test-namespace', got '%s'", event.Name)
		}
		// Cluster-scoped resources should have empty namespace
		if event.Namespace != "" {
			t.Errorf("expected empty namespace for cluster-scoped resource, got '%s'", event.Namespace)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for add event")
	}
}

// TestGenericIngester_SkipSameResourceVersion verifies that updates with same resource version are skipped.
func TestGenericIngester_SkipSameResourceVersion(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	ingester := NewGenericIngester(PodConfig(factory), cfg, log)
	if err := ingester.RegisterHandlers(); err != nil {
		t.Fatalf("failed to register handlers: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid-123",
			ResourceVersion: "1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
		},
	}

	// Directly call the handlers to test resource version check
	ingester.onAdd(pod)

	// Consume the add event
	select {
	case <-eventChan:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for add event")
	}

	// Update with same resource version should be skipped
	podCopy := pod.DeepCopy()
	podCopy.Labels = map[string]string{"changed": "true"}
	// ResourceVersion is still "1"

	ingester.onUpdate(pod, podCopy)

	// Should not receive an event
	select {
	case event := <-eventChan:
		t.Errorf("unexpected event received: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no event should be sent
	}

	// Update with different resource version should send event
	podCopy.ResourceVersion = "2"
	ingester.onUpdate(pod, podCopy)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("expected Modified event, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for update event with new resource version")
	}
}

// TestGenericIngester_ChannelFull verifies behavior when the event channel is full.
func TestGenericIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	// Create a channel with zero buffer - will block immediately
	eventChan := make(chan ResourceEvent)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	ingester := NewGenericIngester(PodConfig(factory), cfg, log)
	if err := ingester.RegisterHandlers(); err != nil {
		t.Fatalf("failed to register handlers: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Directly call onAdd - this should log a warning about channel full, not block
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid-123",
			ResourceVersion: "1",
		},
	}

	done := make(chan struct{})
	go func() {
		ingester.onAdd(pod)
		close(done)
	}()

	// Should complete quickly (not block)
	select {
	case <-done:
		// Success - the add handler returned without blocking
	case <-time.After(time.Second):
		t.Fatal("handler blocked when channel was full")
	}
}

// TestGenericIngester_DeletedFinalStateUnknown tests handling of tombstone objects.
func TestGenericIngester_DeletedFinalStateUnknown(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	ingester := NewGenericIngester(PodConfig(factory), cfg, log)

	// Directly call onDelete with a DeletedFinalStateUnknown object
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "tombstone-pod",
			Namespace:       "default",
			UID:             "tombstone-uid",
			ResourceVersion: "1",
		},
	}

	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/tombstone-pod",
		Obj: pod,
	}

	ingester.onDelete(tombstone)

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("expected Deleted event, got %s", event.Type)
		}
		if event.Name != "tombstone-pod" {
			t.Errorf("expected name 'tombstone-pod', got '%s'", event.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for delete event")
	}
}

// TestGenericIngester_InvalidObjectType tests that invalid objects are handled gracefully.
func TestGenericIngester_InvalidObjectType(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	ingester := NewGenericIngester(PodConfig(factory), cfg, log)

	// Pass a non-Pod object to the Pod ingester - should be handled gracefully
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wrong-type",
		},
	}

	// These should not panic, just log an error
	ingester.onAdd(ns)
	ingester.onUpdate(ns, ns)
	ingester.onDelete(ns)

	// Should not receive any events
	select {
	case event := <-eventChan:
		t.Errorf("unexpected event received: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no events for wrong type
	}
}

// TestIngestersFromFactory verifies that all 20 ingesters are created.
func TestIngestersFromFactory(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	ingesters := IngestersFromFactory(factory, cfg, log)

	// Should create exactly 20 ingesters
	expectedCount := 20
	if len(ingesters) != expectedCount {
		t.Errorf("expected %d ingesters, got %d", expectedCount, len(ingesters))
	}

	// All ingesters should implement the Ingester interface and be non-nil
	for i, ingester := range ingesters {
		if ingester == nil {
			t.Errorf("ingester %d is nil", i)
		}
	}

	// Register all handlers - should not error
	for i, ingester := range ingesters {
		if err := ingester.RegisterHandlers(); err != nil {
			t.Errorf("failed to register handlers for ingester %d: %v", i, err)
		}
	}
}

// TestGenericIngester_AllResourceTypes verifies each resource type can be instantiated and registered.
func TestGenericIngester_AllResourceTypes(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	eventChan := make(chan ResourceEvent, 10)
	cfg := IngesterConfig{EventChan: eventChan}
	log := testr.New(t)

	tests := []struct {
		name         string
		createConfig func() interface{ RegisterHandlers() error }
		resourceType transport.ResourceType
		isNamespaced bool
	}{
		{"Pod", func() interface{ RegisterHandlers() error } { return NewGenericIngester(PodConfig(factory), cfg, log) }, transport.ResourceTypePod, true},
		{"Service", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(ServiceConfig(factory), cfg, log)
		}, transport.ResourceTypeService, true},
		{"Namespace", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(NamespaceConfig(factory), cfg, log)
		}, transport.ResourceTypeNamespace, false},
		{"Node", func() interface{ RegisterHandlers() error } { return NewGenericIngester(NodeConfig(factory), cfg, log) }, transport.ResourceTypeNode, false},
		{"ConfigMap", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(ConfigMapConfig(factory), cfg, log)
		}, transport.ResourceTypeConfigMap, true},
		{"Secret", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(SecretConfig(factory), cfg, log)
		}, transport.ResourceTypeSecret, true},
		{"PVC", func() interface{ RegisterHandlers() error } { return NewGenericIngester(PVCConfig(factory), cfg, log) }, transport.ResourceTypePersistentVolumeClaim, true},
		{"ServiceAccount", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(ServiceAccountConfig(factory), cfg, log)
		}, transport.ResourceTypeServiceAccount, true},
		{"Event", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(EventConfig(factory), cfg, log)
		}, transport.ResourceTypeEvent, true},
		{"Deployment", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(DeploymentConfig(factory), cfg, log)
		}, transport.ResourceTypeDeployment, true},
		{"StatefulSet", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(StatefulSetConfig(factory), cfg, log)
		}, transport.ResourceTypeStatefulSet, true},
		{"DaemonSet", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(DaemonSetConfig(factory), cfg, log)
		}, transport.ResourceTypeDaemonSet, true},
		{"Job", func() interface{ RegisterHandlers() error } { return NewGenericIngester(JobConfig(factory), cfg, log) }, transport.ResourceTypeJob, true},
		{"CronJob", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(CronJobConfig(factory), cfg, log)
		}, transport.ResourceTypeCronJob, true},
		{"Ingress", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(IngressConfig(factory), cfg, log)
		}, transport.ResourceTypeIngress, true},
		{"NetworkPolicy", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(NetworkPolicyConfig(factory), cfg, log)
		}, transport.ResourceTypeNetworkPolicy, true},
		{"Role", func() interface{ RegisterHandlers() error } { return NewGenericIngester(RoleConfig(factory), cfg, log) }, transport.ResourceTypeRole, true},
		{"ClusterRole", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(ClusterRoleConfig(factory), cfg, log)
		}, transport.ResourceTypeClusterRole, false},
		{"RoleBinding", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(RoleBindingConfig(factory), cfg, log)
		}, transport.ResourceTypeRoleBinding, true},
		{"ClusterRoleBinding", func() interface{ RegisterHandlers() error } {
			return NewGenericIngester(ClusterRoleBindingConfig(factory), cfg, log)
		}, transport.ResourceTypeClusterRoleBinding, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingester := tt.createConfig()
			if ingester == nil {
				t.Fatal("failed to create ingester")
			}

			if err := ingester.RegisterHandlers(); err != nil {
				t.Errorf("failed to register handlers: %v", err)
			}
		})
	}
}
