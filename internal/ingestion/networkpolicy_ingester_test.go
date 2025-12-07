// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestNetworkPolicyIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNetworkPolicyIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a network policy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "default",
			UID:       "np-uid-123",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "web",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "frontend",
								},
							},
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "database",
								},
							},
						},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "cache",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.NetworkingV1().NetworkPolicies("default").Create(context.TODO(), np, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create network policy: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeNetworkPolicy {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNetworkPolicy)
		}
		if event.Name != "test-netpol" {
			t.Errorf("Name = %v, want %v", event.Name, "test-netpol")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "np-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "np-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify NetworkPolicyInfo data
		npInfo, ok := event.Object.(transport.NetworkPolicyInfo)
		if !ok {
			t.Errorf("Object is not NetworkPolicyInfo: %T", event.Object)
		} else {
			if npInfo.PodSelector["app"] != "web" {
				t.Errorf("PodSelector[app] = %s, want web", npInfo.PodSelector["app"])
			}
			if len(npInfo.PolicyTypes) != 2 {
				t.Errorf("PolicyTypes count = %d, want 2", len(npInfo.PolicyTypes))
			}
			if npInfo.IngressRules != 1 {
				t.Errorf("IngressRules = %d, want 1", npInfo.IngressRules)
			}
			if npInfo.EgressRules != 2 {
				t.Errorf("EgressRules = %d, want 2", npInfo.EgressRules)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestNetworkPolicyIngester_OnUpdate(t *testing.T) {
	// Create initial network policy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-netpol",
			Namespace:       "default",
			UID:             "np-uid-123",
			ResourceVersion: "1",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "web",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	clientset := fake.NewSimpleClientset(np)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNetworkPolicyIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
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

	// Update the network policy
	updatedNP := np.DeepCopy()
	updatedNP.ResourceVersion = "2"
	updatedNP.Spec.PolicyTypes = append(updatedNP.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	_, err := clientset.NetworkingV1().NetworkPolicies("default").Update(context.TODO(), updatedNP, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update network policy: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeNetworkPolicy {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNetworkPolicy)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestNetworkPolicyIngester_OnDelete(t *testing.T) {
	// Create initial network policy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "default",
			UID:       "np-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(np)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNetworkPolicyIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
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

	// Delete the network policy
	err := clientset.NetworkingV1().NetworkPolicies("default").Delete(context.TODO(), "test-netpol", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete network policy: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeNetworkPolicy {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNetworkPolicy)
		}
		if event.Name != "test-netpol" {
			t.Errorf("Name = %v, want %v", event.Name, "test-netpol")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestNetworkPolicyIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewNetworkPolicyIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a network policy - this should not block even though channel is full
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "default",
			UID:       "np-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.NetworkingV1().NetworkPolicies("default").Create(context.TODO(), np, metav1.CreateOptions{})
		close(done)
	}()

	// Should complete without blocking
	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestNetworkPolicyIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial network policy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-netpol",
			Namespace:       "default",
			UID:             "np-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(np)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNetworkPolicyIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
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
	ingester.onUpdate(np, np)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
