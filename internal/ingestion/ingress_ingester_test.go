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

func TestIngressIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewIngressIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create an ingress
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "nginx"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			UID:       "ing-uid-123",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-service",
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{"example.com"},
					SecretName: "tls-secret",
				},
			},
		},
	}

	_, err := clientset.NetworkingV1().Ingresses("default").Create(context.TODO(), ing, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create ingress: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeIngress {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeIngress)
		}
		if event.Name != "test-ingress" {
			t.Errorf("Name = %v, want %v", event.Name, "test-ingress")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "ing-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "ing-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify IngressInfo data
		ingInfo, ok := event.Object.(transport.IngressInfo)
		if !ok {
			t.Errorf("Object is not IngressInfo: %T", event.Object)
		} else {
			if ingInfo.IngressClassName != "nginx" {
				t.Errorf("IngressClassName = %s, want nginx", ingInfo.IngressClassName)
			}
			if len(ingInfo.Rules) != 1 {
				t.Errorf("Rules count = %d, want 1", len(ingInfo.Rules))
			} else if ingInfo.Rules[0].Host != "example.com" {
				t.Errorf("Rules[0].Host = %s, want example.com", ingInfo.Rules[0].Host)
			}
			if len(ingInfo.TLS) != 1 {
				t.Errorf("TLS count = %d, want 1", len(ingInfo.TLS))
			} else if ingInfo.TLS[0].SecretName != "tls-secret" {
				t.Errorf("TLS[0].SecretName = %s, want tls-secret", ingInfo.TLS[0].SecretName)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestIngressIngester_OnUpdate(t *testing.T) {
	// Create initial ingress
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ingress",
			Namespace:       "default",
			UID:             "ing-uid-123",
			ResourceVersion: "1",
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: &pathType,
								},
							},
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewIngressIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

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

	// Update the ingress
	updatedIng := ing.DeepCopy()
	updatedIng.ResourceVersion = "2"
	updatedIng.Spec.Rules[0].Host = "updated.example.com"

	_, err := clientset.NetworkingV1().Ingresses("default").Update(context.TODO(), updatedIng, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update ingress: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeIngress {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeIngress)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestIngressIngester_OnDelete(t *testing.T) {
	// Create initial ingress
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			UID:       "ing-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(ing)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewIngressIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

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

	// Delete the ingress
	err := clientset.NetworkingV1().Ingresses("default").Delete(context.TODO(), "test-ingress", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete ingress: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeIngress {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeIngress)
		}
		if event.Name != "test-ingress" {
			t.Errorf("Name = %v, want %v", event.Name, "test-ingress")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestIngressIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewIngressIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create an ingress - this should not block even though channel is full
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			UID:       "ing-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.NetworkingV1().Ingresses("default").Create(context.TODO(), ing, metav1.CreateOptions{})
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

func TestIngressIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial ingress
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ingress",
			Namespace:       "default",
			UID:             "ing-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(ing)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewIngressIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

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
	ingester.onUpdate(ing, ing)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
