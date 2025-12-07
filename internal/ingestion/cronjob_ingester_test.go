// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestCronJobIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewCronJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a cronjob
	suspend := false
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "default",
			UID:       "cj-uid-123",
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          "*/5 * * * *",
			Suspend:           &suspend,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
		},
	}

	_, err := clientset.BatchV1().CronJobs("default").Create(context.TODO(), cj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create cronjob: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeCronJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeCronJob)
		}
		if event.Name != "test-cronjob" {
			t.Errorf("Name = %v, want %v", event.Name, "test-cronjob")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "cj-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "cj-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify CronJobInfo data
		cjInfo, ok := event.Object.(transport.CronJobInfo)
		if !ok {
			t.Errorf("Object is not CronJobInfo: %T", event.Object)
		} else {
			if cjInfo.Schedule != "*/5 * * * *" {
				t.Errorf("Schedule = %s, want */5 * * * *", cjInfo.Schedule)
			}
			if cjInfo.Suspend != false {
				t.Errorf("Suspend = %v, want false", cjInfo.Suspend)
			}
			if cjInfo.ConcurrencyPolicy != "Forbid" {
				t.Errorf("ConcurrencyPolicy = %s, want Forbid", cjInfo.ConcurrencyPolicy)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestCronJobIngester_OnUpdate(t *testing.T) {
	// Create initial cronjob
	suspend := false
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cronjob",
			Namespace:       "default",
			UID:             "cj-uid-123",
			ResourceVersion: "1",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			Suspend:  &suspend,
		},
	}

	clientset := fake.NewSimpleClientset(cj)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewCronJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the cronjob
	updatedCJ := cj.DeepCopy()
	updatedCJ.ResourceVersion = "2"
	newSuspend := true
	updatedCJ.Spec.Suspend = &newSuspend

	_, err := clientset.BatchV1().CronJobs("default").Update(context.TODO(), updatedCJ, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update cronjob: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeUpdated {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeUpdated)
		}
		if event.Kind != transport.ResourceTypeCronJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeCronJob)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestCronJobIngester_OnDelete(t *testing.T) {
	// Create initial cronjob
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "default",
			UID:       "cj-uid-123",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
		},
	}

	clientset := fake.NewSimpleClientset(cj)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewCronJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the cronjob
	err := clientset.BatchV1().CronJobs("default").Delete(context.TODO(), "test-cronjob", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete cronjob: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeCronJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeCronJob)
		}
		if event.Name != "test-cronjob" {
			t.Errorf("Name = %v, want %v", event.Name, "test-cronjob")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestCronJobIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewCronJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a cronjob - this should not block even though channel is full
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "default",
			UID:       "cj-uid-123",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.BatchV1().CronJobs("default").Create(context.TODO(), cj, metav1.CreateOptions{})
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

func TestCronJobIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial cronjob
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cronjob",
			Namespace:       "default",
			UID:             "cj-uid-123",
			ResourceVersion: "1",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
		},
	}

	clientset := fake.NewSimpleClientset(cj)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewCronJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(cj, cj)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
