// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestClusterRoleIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-clusterrole",
			UID:    "cr-uid-123",
			Labels: map[string]string{"app": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes", "namespaces"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "statefulsets"},
				Verbs:     []string{"*"},
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoles().Create(context.Background(), clusterRole, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create clusterrole: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeClusterRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRole)
		}
		if event.Name != "test-clusterrole" {
			t.Errorf("Name = %v, want %v", event.Name, "test-clusterrole")
		}
		if event.Namespace != "" {
			t.Errorf("Namespace = %v, want empty string for cluster-scoped resource", event.Namespace)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		roleInfo, ok := event.Object.(transport.RoleInfo)
		if !ok {
			t.Errorf("Object is not RoleInfo: %T", event.Object)
		} else {
			if roleInfo.RuleCount != 2 {
				t.Errorf("RuleCount = %d, want 2", roleInfo.RuleCount)
			}
			if !roleInfo.IsCluster {
				t.Error("IsCluster should be true for ClusterRole")
			}
			if len(roleInfo.Resources) != 4 {
				t.Errorf("Resources count = %d, want 4", len(roleInfo.Resources))
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestClusterRoleIngester_OnUpdate(t *testing.T) {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-clusterrole",
			UID:             "cr-uid-123",
			ResourceVersion: "1",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(clusterRole)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the clusterrole
	updatedCR := clusterRole.DeepCopy()
	updatedCR.ResourceVersion = "2"
	updatedCR.Rules = append(updatedCR.Rules, rbacv1.PolicyRule{
		APIGroups: []string{"apps"},
		Resources: []string{"deployments"},
		Verbs:     []string{"get", "list"},
	})

	_, err := clientset.RbacV1().ClusterRoles().Update(context.Background(), updatedCR, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update clusterrole: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeClusterRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRole)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestClusterRoleIngester_OnDelete(t *testing.T) {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterrole",
			UID:  "cr-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(clusterRole)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the clusterrole
	err := clientset.RbacV1().ClusterRoles().Delete(context.Background(), "test-clusterrole", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete clusterrole: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeClusterRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRole)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestClusterRoleIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterrole",
			UID:  "cr-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.RbacV1().ClusterRoles().Create(context.Background(), clusterRole, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestClusterRoleIngester_SkipSameResourceVersion(t *testing.T) {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-clusterrole",
			UID:             "cr-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(clusterRole)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(clusterRole, clusterRole)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
