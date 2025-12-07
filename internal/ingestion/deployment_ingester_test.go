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

func TestDeploymentIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDeploymentIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a deployment
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			UID:       "deploy-uid-123",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     2,
			AvailableReplicas: 2,
			UpdatedReplicas:   3,
		},
	}

	_, err := clientset.AppsV1().Deployments("default").Create(context.TODO(), deploy, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeDeployment {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDeployment)
		}
		if event.Name != "test-deployment" {
			t.Errorf("Name = %v, want %v", event.Name, "test-deployment")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "deploy-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "deploy-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify DeploymentInfo data
		deployInfo, ok := event.Object.(transport.DeploymentInfo)
		if !ok {
			t.Errorf("Object is not DeploymentInfo: %T", event.Object)
		} else {
			if deployInfo.Replicas != 3 {
				t.Errorf("Replicas = %d, want 3", deployInfo.Replicas)
			}
			if deployInfo.Image != "nginx:latest" {
				t.Errorf("Image = %s, want nginx:latest", deployInfo.Image)
			}
			if deployInfo.Strategy != "RollingUpdate" {
				t.Errorf("Strategy = %s, want RollingUpdate", deployInfo.Strategy)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestDeploymentIngester_OnUpdate(t *testing.T) {
	// Create initial deployment
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deployment",
			Namespace:       "default",
			UID:             "deploy-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(deploy)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDeploymentIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the deployment
	updatedDeploy := deploy.DeepCopy()
	updatedDeploy.ResourceVersion = "2"
	newReplicas := int32(5)
	updatedDeploy.Spec.Replicas = &newReplicas

	_, err := clientset.AppsV1().Deployments("default").Update(context.TODO(), updatedDeploy, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update deployment: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeDeployment {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDeployment)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestDeploymentIngester_OnDelete(t *testing.T) {
	// Create initial deployment
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			UID:       "deploy-uid-123",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(deploy)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDeploymentIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the deployment
	err := clientset.AppsV1().Deployments("default").Delete(context.TODO(), "test-deployment", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete deployment: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeDeployment {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeDeployment)
		}
		if event.Name != "test-deployment" {
			t.Errorf("Name = %v, want %v", event.Name, "test-deployment")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestDeploymentIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewDeploymentIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a deployment - this should not block even though channel is full
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			UID:       "deploy-uid-123",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.AppsV1().Deployments("default").Create(context.TODO(), deploy, metav1.CreateOptions{})
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

func TestDeploymentIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial deployment
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deployment",
			Namespace:       "default",
			UID:             "deploy-uid-123",
			ResourceVersion: "1",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(deploy)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewDeploymentIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(deploy, deploy)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestNewDeploymentInfo(t *testing.T) {
	testCases := []struct {
		name             string
		deploy           *appsv1.Deployment
		expectedReplicas int32
		expectedImage    string
		expectedStrategy string
	}{
		{
			name: "deployment with all fields",
			deploy: func() *appsv1.Deployment {
				replicas := int32(5)
				return &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
						UID:       "uid-123",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Strategy: appsv1.DeploymentStrategy{
							Type: appsv1.RollingUpdateDeploymentStrategyType,
						},
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "main", Image: "nginx:1.19"},
								},
							},
						},
					},
				}
			}(),
			expectedReplicas: 5,
			expectedImage:    "nginx:1.19",
			expectedStrategy: "RollingUpdate",
		},
		{
			name: "deployment with nil replicas (defaults to 1)",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Image: "app:latest"},
							},
						},
					},
				},
			},
			expectedReplicas: 1,
			expectedImage:    "app:latest",
			expectedStrategy: "",
		},
		{
			name: "deployment with no containers",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
			expectedReplicas: 1,
			expectedImage:    "",
			expectedStrategy: "",
		},
		{
			name: "deployment with Recreate strategy",
			deploy: func() *appsv1.Deployment {
				replicas := int32(2)
				return &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Strategy: appsv1.DeploymentStrategy{
							Type: appsv1.RecreateDeploymentStrategyType,
						},
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "main", Image: "app:v2"},
								},
							},
						},
					},
				}
			}(),
			expectedReplicas: 2,
			expectedImage:    "app:v2",
			expectedStrategy: "Recreate",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := transport.NewDeploymentInfo(tc.deploy)

			if info.Replicas != tc.expectedReplicas {
				t.Errorf("Replicas = %d, want %d", info.Replicas, tc.expectedReplicas)
			}
			if info.Image != tc.expectedImage {
				t.Errorf("Image = %s, want %s", info.Image, tc.expectedImage)
			}
			if info.Strategy != tc.expectedStrategy {
				t.Errorf("Strategy = %s, want %s", info.Strategy, tc.expectedStrategy)
			}
		})
	}
}
