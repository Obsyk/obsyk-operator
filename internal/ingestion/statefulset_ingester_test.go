// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestStatefulSetIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewStatefulSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a statefulset
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
			UID:       "sts-uid-123",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   2,
			CurrentReplicas: 3,
		},
	}

	_, err := clientset.AppsV1().StatefulSets("default").Create(context.TODO(), sts, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create statefulset: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeStatefulSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeStatefulSet)
		}
		if event.Name != "test-statefulset" {
			t.Errorf("Name = %v, want %v", event.Name, "test-statefulset")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "sts-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "sts-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify StatefulSetInfo data
		stsInfo, ok := event.Object.(transport.StatefulSetInfo)
		if !ok {
			t.Errorf("Object is not StatefulSetInfo: %T", event.Object)
		} else {
			if stsInfo.Replicas != 3 {
				t.Errorf("Replicas = %d, want 3", stsInfo.Replicas)
			}
			if stsInfo.ServiceName != "test-headless" {
				t.Errorf("ServiceName = %s, want test-headless", stsInfo.ServiceName)
			}
			if stsInfo.Image != "postgres:14" {
				t.Errorf("Image = %s, want postgres:14", stsInfo.Image)
			}
			if stsInfo.UpdateStrategy != "RollingUpdate" {
				t.Errorf("UpdateStrategy = %s, want RollingUpdate", stsInfo.UpdateStrategy)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestStatefulSetIngester_OnUpdate(t *testing.T) {
	// Create initial statefulset
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-statefulset",
			Namespace:       "default",
			UID:             "sts-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(sts)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewStatefulSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the statefulset
	updatedSts := sts.DeepCopy()
	updatedSts.ResourceVersion = "2"
	newReplicas := int32(5)
	updatedSts.Spec.Replicas = &newReplicas

	_, err := clientset.AppsV1().StatefulSets("default").Update(context.TODO(), updatedSts, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update statefulset: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeStatefulSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeStatefulSet)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestStatefulSetIngester_OnDelete(t *testing.T) {
	// Create initial statefulset
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
			UID:       "sts-uid-123",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(sts)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewStatefulSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the statefulset
	err := clientset.AppsV1().StatefulSets("default").Delete(context.TODO(), "test-statefulset", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete statefulset: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeStatefulSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeStatefulSet)
		}
		if event.Name != "test-statefulset" {
			t.Errorf("Name = %v, want %v", event.Name, "test-statefulset")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestStatefulSetIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewStatefulSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a statefulset - this should not block even though channel is full
	replicas := int32(1)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
			UID:       "sts-uid-123",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.AppsV1().StatefulSets("default").Create(context.TODO(), sts, metav1.CreateOptions{})
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

func TestStatefulSetIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial statefulset
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-statefulset",
			Namespace:       "default",
			UID:             "sts-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(sts)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewStatefulSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(sts, sts)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestNewStatefulSetInfo(t *testing.T) {
	testCases := []struct {
		name             string
		sts              *appsv1.StatefulSet
		expectedReplicas int32
		expectedImage    string
		expectedStrategy string
		expectedSvcName  string
	}{
		{
			name: "statefulset with all fields",
			sts: func() *appsv1.StatefulSet {
				replicas := int32(5)
				return &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
						UID:       "uid-123",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas:    &replicas,
						ServiceName: "test-headless",
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
						},
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "main", Image: "postgres:14"},
								},
							},
						},
					},
				}
			}(),
			expectedReplicas: 5,
			expectedImage:    "postgres:14",
			expectedStrategy: "RollingUpdate",
			expectedSvcName:  "test-headless",
		},
		{
			name: "statefulset with nil replicas (defaults to 1)",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: "test-svc",
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Image: "mysql:8"},
							},
						},
					},
				},
			},
			expectedReplicas: 1,
			expectedImage:    "mysql:8",
			expectedStrategy: "",
			expectedSvcName:  "test-svc",
		},
		{
			name: "statefulset with OnDelete strategy",
			sts: func() *appsv1.StatefulSet {
				replicas := int32(2)
				return &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas:    &replicas,
						ServiceName: "test-svc",
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.OnDeleteStatefulSetStrategyType,
						},
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "main", Image: "redis:7"},
								},
							},
						},
					},
				}
			}(),
			expectedReplicas: 2,
			expectedImage:    "redis:7",
			expectedStrategy: "OnDelete",
			expectedSvcName:  "test-svc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := transport.NewStatefulSetInfo(tc.sts)

			if info.Replicas != tc.expectedReplicas {
				t.Errorf("Replicas = %d, want %d", info.Replicas, tc.expectedReplicas)
			}
			if info.Image != tc.expectedImage {
				t.Errorf("Image = %s, want %s", info.Image, tc.expectedImage)
			}
			if info.UpdateStrategy != tc.expectedStrategy {
				t.Errorf("UpdateStrategy = %s, want %s", info.UpdateStrategy, tc.expectedStrategy)
			}
			if info.ServiceName != tc.expectedSvcName {
				t.Errorf("ServiceName = %s, want %s", info.ServiceName, tc.expectedSvcName)
			}
		})
	}
}
