// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestPVCIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a PVC
	storageClass := "fast-ssd"
	volumeMode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "pvc-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       &volumeMode,
			VolumeName:       "pv-123",
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	_, err := clientset.CoreV1().PersistentVolumeClaims("default").Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create PVC: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypePersistentVolumeClaim {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypePersistentVolumeClaim)
		}
		if event.Name != "test-pvc" {
			t.Errorf("Name = %v, want %v", event.Name, "test-pvc")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "pvc-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "pvc-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify PVCInfo data
		pvcInfo, ok := event.Object.(transport.PVCInfo)
		if !ok {
			t.Errorf("Object is not PVCInfo: %T", event.Object)
		} else {
			if pvcInfo.StorageClassName != "fast-ssd" {
				t.Errorf("StorageClassName = %s, want fast-ssd", pvcInfo.StorageClassName)
			}
			if len(pvcInfo.AccessModes) != 1 || pvcInfo.AccessModes[0] != "ReadWriteOnce" {
				t.Errorf("AccessModes = %v, want [ReadWriteOnce]", pvcInfo.AccessModes)
			}
			if pvcInfo.StorageRequest != "10Gi" {
				t.Errorf("StorageRequest = %s, want 10Gi", pvcInfo.StorageRequest)
			}
			if pvcInfo.VolumeName != "pv-123" {
				t.Errorf("VolumeName = %s, want pv-123", pvcInfo.VolumeName)
			}
			if pvcInfo.VolumeMode != "Filesystem" {
				t.Errorf("VolumeMode = %s, want Filesystem", pvcInfo.VolumeMode)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestPVCIngester_OnUpdate(t *testing.T) {
	// Create initial PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pvc",
			Namespace:       "default",
			UID:             "pvc-uid-123",
			ResourceVersion: "1",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	clientset := fake.NewSimpleClientset(pvc)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the PVC
	updatedPVC := pvc.DeepCopy()
	updatedPVC.ResourceVersion = "2"
	updatedPVC.Spec.VolumeName = "pv-123"
	updatedPVC.Status.Phase = corev1.ClaimBound

	_, err := clientset.CoreV1().PersistentVolumeClaims("default").Update(context.TODO(), updatedPVC, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update PVC: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypePersistentVolumeClaim {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypePersistentVolumeClaim)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
		// Verify updated data
		pvcInfo, ok := event.Object.(transport.PVCInfo)
		if !ok {
			t.Errorf("Object is not PVCInfo: %T", event.Object)
		} else {
			if pvcInfo.Phase != "Bound" {
				t.Errorf("Phase = %s, want Bound", pvcInfo.Phase)
			}
			if pvcInfo.VolumeName != "pv-123" {
				t.Errorf("VolumeName = %s, want pv-123", pvcInfo.VolumeName)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestPVCIngester_OnDelete(t *testing.T) {
	// Create initial PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "pvc-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(pvc)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the PVC
	err := clientset.CoreV1().PersistentVolumeClaims("default").Delete(context.TODO(), "test-pvc", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete PVC: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypePersistentVolumeClaim {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypePersistentVolumeClaim)
		}
		if event.Name != "test-pvc" {
			t.Errorf("Name = %v, want %v", event.Name, "test-pvc")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestPVCIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a PVC - this should not block even though channel is full
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "pvc-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.CoreV1().PersistentVolumeClaims("default").Create(context.TODO(), pvc, metav1.CreateOptions{})
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

func TestPVCIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pvc",
			Namespace:       "default",
			UID:             "pvc-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(pvc)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(pvc, pvc)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestPVCIngester_PVCPhases(t *testing.T) {
	testCases := []struct {
		name     string
		phase    corev1.PersistentVolumeClaimPhase
		expected string
	}{
		{
			name:     "Pending",
			phase:    corev1.ClaimPending,
			expected: "Pending",
		},
		{
			name:     "Bound",
			phase:    corev1.ClaimBound,
			expected: "Bound",
		},
		{
			name:     "Lost",
			phase:    corev1.ClaimLost,
			expected: "Lost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			factory := informers.NewSharedInformerFactory(clientset, 0)
			eventChan := make(chan ResourceEvent, 10)
			log := ctrl.Log.WithName("test")

			ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
			_ = ingester.RegisterHandlers()

			// Start informer
			stopCh := make(chan struct{})
			defer close(stopCh)
			factory.Start(stopCh)
			factory.WaitForCacheSync(stopCh)

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc-" + tc.name,
					Namespace: "default",
					UID:       types.UID("pvc-uid-" + tc.name),
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: tc.phase,
				},
			}

			_, err := clientset.CoreV1().PersistentVolumeClaims("default").Create(context.TODO(), pvc, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("failed to create PVC: %v", err)
			}

			select {
			case event := <-eventChan:
				pvcInfo, ok := event.Object.(transport.PVCInfo)
				if !ok {
					t.Fatal("Object is not PVCInfo")
				}
				if pvcInfo.Phase != tc.expected {
					t.Errorf("Phase = %s, want %s", pvcInfo.Phase, tc.expected)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for event")
			}
		})
	}
}

func TestPVCIngester_AccessModes(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewPVCIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "pvc-uid-123",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
				corev1.ReadOnlyMany,
			},
		},
	}

	_, err := clientset.CoreV1().PersistentVolumeClaims("default").Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create PVC: %v", err)
	}

	select {
	case event := <-eventChan:
		pvcInfo, ok := event.Object.(transport.PVCInfo)
		if !ok {
			t.Fatal("Object is not PVCInfo")
		}
		if len(pvcInfo.AccessModes) != 2 {
			t.Errorf("AccessModes count = %d, want 2", len(pvcInfo.AccessModes))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}
