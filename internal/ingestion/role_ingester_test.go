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

func TestRoleIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
			UID:       "role-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "services"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	_, err := clientset.RbacV1().Roles("default").Create(context.Background(), role, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRole)
		}
		if event.Name != "test-role" {
			t.Errorf("Name = %v, want %v", event.Name, "test-role")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
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
			if roleInfo.IsCluster {
				t.Error("IsCluster should be false for Role")
			}
			if len(roleInfo.Resources) != 3 {
				t.Errorf("Resources count = %d, want 3", len(roleInfo.Resources))
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestRoleIngester_OnUpdate(t *testing.T) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			Namespace:       "default",
			UID:             "role-uid-123",
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

	clientset := fake.NewSimpleClientset(role)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the role
	updatedRole := role.DeepCopy()
	updatedRole.ResourceVersion = "2"
	updatedRole.Rules = append(updatedRole.Rules, rbacv1.PolicyRule{
		APIGroups: []string{"apps"},
		Resources: []string{"deployments"},
		Verbs:     []string{"get", "list"},
	})

	_, err := clientset.RbacV1().Roles("default").Update(context.Background(), updatedRole, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update role: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRole)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestRoleIngester_OnDelete(t *testing.T) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
			UID:       "role-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(role)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the role
	err := clientset.RbacV1().Roles("default").Delete(context.Background(), "test-role", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete role: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeRole {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRole)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestRoleIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
			UID:       "role-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.RbacV1().Roles("default").Create(context.Background(), role, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestRoleIngester_SkipSameResourceVersion(t *testing.T) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			Namespace:       "default",
			UID:             "role-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(role)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(role, role)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
