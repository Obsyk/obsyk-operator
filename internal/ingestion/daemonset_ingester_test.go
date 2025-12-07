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

func TestDaemonSetIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDaemonSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
			UID:       "ds-uid-123",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
					NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            4,
			NumberAvailable:        4,
		},
	}

	_, err := clientset.AppsV1().DaemonSets("default").Create(context.TODO(), ds, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create daemonset: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeDaemonSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDaemonSet)
		}
		if event.Name != "test-daemonset" {
			t.Errorf("Name = %v, want %v", event.Name, "test-daemonset")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "ds-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "ds-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify DaemonSetInfo data
		dsInfo, ok := event.Object.(transport.DaemonSetInfo)
		if !ok {
			t.Errorf("Object is not DaemonSetInfo: %T", event.Object)
		} else {
			if dsInfo.DesiredNumberScheduled != 5 {
				t.Errorf("DesiredNumberScheduled = %d, want 5", dsInfo.DesiredNumberScheduled)
			}
			if dsInfo.NumberReady != 4 {
				t.Errorf("NumberReady = %d, want 4", dsInfo.NumberReady)
			}
			if dsInfo.Image != "fluentd:v1.14" {
				t.Errorf("Image = %s, want fluentd:v1.14", dsInfo.Image)
			}
			if dsInfo.UpdateStrategy != "RollingUpdate" {
				t.Errorf("UpdateStrategy = %s, want RollingUpdate", dsInfo.UpdateStrategy)
			}
			if dsInfo.NodeSelector["kubernetes.io/os"] != "linux" {
				t.Errorf("NodeSelector = %v, want kubernetes.io/os=linux", dsInfo.NodeSelector)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestDaemonSetIngester_OnUpdate(t *testing.T) {
	// Create initial daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-daemonset",
			Namespace:       "default",
			UID:             "ds-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ds)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDaemonSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the daemonset
	updatedDs := ds.DeepCopy()
	updatedDs.ResourceVersion = "2"
	updatedDs.Spec.Template.Spec.Containers[0].Image = "fluentd:v1.15"

	_, err := clientset.AppsV1().DaemonSets("default").Update(context.TODO(), updatedDs, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update daemonset: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeDaemonSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDaemonSet)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestDaemonSetIngester_OnDelete(t *testing.T) {
	// Create initial daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
			UID:       "ds-uid-123",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ds)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDaemonSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the daemonset
	err := clientset.AppsV1().DaemonSets("default").Delete(context.TODO(), "test-daemonset", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete daemonset: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeDaemonSet {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDaemonSet)
		}
		if event.Name != "test-daemonset" {
			t.Errorf("Name = %v, want %v", event.Name, "test-daemonset")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestDaemonSetIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewDaemonSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a daemonset - this should not block even though channel is full
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
			UID:       "ds-uid-123",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
				},
			},
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.AppsV1().DaemonSets("default").Create(context.TODO(), ds, metav1.CreateOptions{})
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

func TestDaemonSetIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-daemonset",
			Namespace:       "default",
			UID:             "ds-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ds)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDaemonSetIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(ds, ds)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestNewDaemonSetInfo(t *testing.T) {
	testCases := []struct {
		name                       string
		ds                         *appsv1.DaemonSet
		expectedDesiredScheduled   int32
		expectedNumberReady        int32
		expectedImage              string
		expectedStrategy           string
		expectedNodeSelectorLength int
	}{
		{
			name: "daemonset with all fields",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					UID:       "uid-123",
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Image: "fluentd:v1.14"},
							},
							NodeSelector: map[string]string{
								"kubernetes.io/os": "linux",
							},
						},
					},
				},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 10,
					CurrentNumberScheduled: 10,
					NumberReady:            8,
					NumberAvailable:        8,
				},
			},
			expectedDesiredScheduled:   10,
			expectedNumberReady:        8,
			expectedImage:              "fluentd:v1.14",
			expectedStrategy:           "RollingUpdate",
			expectedNodeSelectorLength: 1,
		},
		{
			name: "daemonset with OnDelete strategy",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.OnDeleteDaemonSetStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Image: "node-exporter:v1"},
							},
						},
					},
				},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 5,
					NumberReady:            5,
				},
			},
			expectedDesiredScheduled:   5,
			expectedNumberReady:        5,
			expectedImage:              "node-exporter:v1",
			expectedStrategy:           "OnDelete",
			expectedNodeSelectorLength: 0,
		},
		{
			name: "daemonset with no containers",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
			expectedDesiredScheduled:   0,
			expectedNumberReady:        0,
			expectedImage:              "",
			expectedStrategy:           "",
			expectedNodeSelectorLength: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := transport.NewDaemonSetInfo(tc.ds)

			if info.DesiredNumberScheduled != tc.expectedDesiredScheduled {
				t.Errorf("DesiredNumberScheduled = %d, want %d", info.DesiredNumberScheduled, tc.expectedDesiredScheduled)
			}
			if info.NumberReady != tc.expectedNumberReady {
				t.Errorf("NumberReady = %d, want %d", info.NumberReady, tc.expectedNumberReady)
			}
			if info.Image != tc.expectedImage {
				t.Errorf("Image = %s, want %s", info.Image, tc.expectedImage)
			}
			if info.UpdateStrategy != tc.expectedStrategy {
				t.Errorf("UpdateStrategy = %s, want %s", info.UpdateStrategy, tc.expectedStrategy)
			}
			if len(info.NodeSelector) != tc.expectedNodeSelectorLength {
				t.Errorf("NodeSelector length = %d, want %d", len(info.NodeSelector), tc.expectedNodeSelectorLength)
			}
		})
	}
}
