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

func TestRoleBindingIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rolebinding",
			Namespace: "default",
			UID:       "rb-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "test-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: "default",
			},
			{
				Kind: "User",
				Name: "jane",
			},
		},
	}

	_, err := clientset.RbacV1().RoleBindings("default").Create(context.Background(), rb, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create rolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRoleBinding)
		}
		if event.Name != "test-rolebinding" {
			t.Errorf("Name = %v, want %v", event.Name, "test-rolebinding")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		rbInfo, ok := event.Object.(transport.RoleBindingInfo)
		if !ok {
			t.Errorf("Object is not RoleBindingInfo: %T", event.Object)
		} else {
			if rbInfo.RoleRef.Kind != "Role" {
				t.Errorf("RoleRef.Kind = %s, want Role", rbInfo.RoleRef.Kind)
			}
			if rbInfo.RoleRef.Name != "test-role" {
				t.Errorf("RoleRef.Name = %s, want test-role", rbInfo.RoleRef.Name)
			}
			if len(rbInfo.Subjects) != 2 {
				t.Errorf("Subjects count = %d, want 2", len(rbInfo.Subjects))
			}
			if rbInfo.IsCluster {
				t.Error("IsCluster should be false for RoleBinding")
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestRoleBindingIngester_OnUpdate(t *testing.T) {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rolebinding",
			Namespace:       "default",
			UID:             "rb-uid-123",
			ResourceVersion: "1",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "test-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: "default",
			},
		},
	}

	clientset := fake.NewSimpleClientset(rb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

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

	// Update the rolebinding
	updatedRB := rb.DeepCopy()
	updatedRB.ResourceVersion = "2"
	updatedRB.Subjects = append(updatedRB.Subjects, rbacv1.Subject{
		Kind: "User",
		Name: "newuser",
	})

	_, err := clientset.RbacV1().RoleBindings("default").Update(context.Background(), updatedRB, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update rolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRoleBinding)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestRoleBindingIngester_OnDelete(t *testing.T) {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rolebinding",
			Namespace: "default",
			UID:       "rb-uid-123",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "Role",
			Name: "test-role",
		},
	}

	clientset := fake.NewSimpleClientset(rb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

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

	// Delete the rolebinding
	err := clientset.RbacV1().RoleBindings("default").Delete(context.Background(), "test-rolebinding", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete rolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeRoleBinding)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestRoleBindingIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rolebinding",
			Namespace: "default",
			UID:       "rb-uid-123",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "Role",
			Name: "test-role",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.RbacV1().RoleBindings("default").Create(context.Background(), rb, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestRoleBindingIngester_SkipSameResourceVersion(t *testing.T) {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rolebinding",
			Namespace:       "default",
			UID:             "rb-uid-123",
			ResourceVersion: "1",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "Role",
			Name: "test-role",
		},
	}

	clientset := fake.NewSimpleClientset(rb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

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
	ingester.onUpdate(rb, rb)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
