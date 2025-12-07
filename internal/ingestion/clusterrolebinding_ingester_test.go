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

func TestClusterRoleBindingIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-clusterrolebinding",
			UID:    "crb-uid-123",
			Labels: map[string]string{"app": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "admin-sa",
				Namespace: "kube-system",
			},
			{
				Kind: "Group",
				Name: "system:masters",
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create clusterrolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeClusterRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRoleBinding)
		}
		if event.Name != "test-clusterrolebinding" {
			t.Errorf("Name = %v, want %v", event.Name, "test-clusterrolebinding")
		}
		if event.Namespace != "" {
			t.Errorf("Namespace = %v, want empty string for cluster-scoped resource", event.Namespace)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		rbInfo, ok := event.Object.(transport.RoleBindingInfo)
		if !ok {
			t.Errorf("Object is not RoleBindingInfo: %T", event.Object)
		} else {
			if rbInfo.RoleRef.Kind != "ClusterRole" {
				t.Errorf("RoleRef.Kind = %s, want ClusterRole", rbInfo.RoleRef.Kind)
			}
			if rbInfo.RoleRef.Name != "cluster-admin" {
				t.Errorf("RoleRef.Name = %s, want cluster-admin", rbInfo.RoleRef.Name)
			}
			if len(rbInfo.Subjects) != 2 {
				t.Errorf("Subjects count = %d, want 2", len(rbInfo.Subjects))
			}
			if !rbInfo.IsCluster {
				t.Error("IsCluster should be true for ClusterRoleBinding")
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestClusterRoleBindingIngester_OnUpdate(t *testing.T) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-clusterrolebinding",
			UID:             "crb-uid-123",
			ResourceVersion: "1",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "admin-sa",
				Namespace: "kube-system",
			},
		},
	}

	clientset := fake.NewSimpleClientset(crb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the clusterrolebinding
	updatedCRB := crb.DeepCopy()
	updatedCRB.ResourceVersion = "2"
	updatedCRB.Subjects = append(updatedCRB.Subjects, rbacv1.Subject{
		Kind: "User",
		Name: "newadmin",
	})

	_, err := clientset.RbacV1().ClusterRoleBindings().Update(context.Background(), updatedCRB, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update clusterrolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeClusterRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRoleBinding)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestClusterRoleBindingIngester_OnDelete(t *testing.T) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterrolebinding",
			UID:  "crb-uid-123",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "cluster-admin",
		},
	}

	clientset := fake.NewSimpleClientset(crb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the clusterrolebinding
	err := clientset.RbacV1().ClusterRoleBindings().Delete(context.Background(), "test-clusterrolebinding", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete clusterrolebinding: %v", err)
	}

	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeClusterRoleBinding {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeClusterRoleBinding)
		}
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestClusterRoleBindingIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterrolebinding",
			UID:  "crb-uid-123",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "cluster-admin",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
		close(done)
	}()

	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestClusterRoleBindingIngester_SkipSameResourceVersion(t *testing.T) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-clusterrolebinding",
			UID:             "crb-uid-123",
			ResourceVersion: "1",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "cluster-admin",
		},
	}

	clientset := fake.NewSimpleClientset(crb)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewClusterRoleBindingIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(crb, crb)

	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
