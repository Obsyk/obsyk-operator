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

func TestJobIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a job
	completions := int32(3)
	parallelism := int32(2)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid-123",
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
			Parallelism: &parallelism,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
			Failed:    0,
			Active:    2,
		},
	}

	_, err := clientset.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeJob)
		}
		if event.Name != "test-job" {
			t.Errorf("Name = %v, want %v", event.Name, "test-job")
		}
		if event.Namespace != "default" {
			t.Errorf("Namespace = %v, want %v", event.Namespace, "default")
		}
		if event.UID != "job-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "job-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify JobInfo data
		jobInfo, ok := event.Object.(transport.JobInfo)
		if !ok {
			t.Errorf("Object is not JobInfo: %T", event.Object)
		} else {
			if jobInfo.Completions != 3 {
				t.Errorf("Completions = %d, want 3", jobInfo.Completions)
			}
			if jobInfo.Parallelism != 2 {
				t.Errorf("Parallelism = %d, want 2", jobInfo.Parallelism)
			}
			if jobInfo.Succeeded != 1 {
				t.Errorf("Succeeded = %d, want 1", jobInfo.Succeeded)
			}
			if jobInfo.Active != 2 {
				t.Errorf("Active = %d, want 2", jobInfo.Active)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestJobIngester_OnUpdate(t *testing.T) {
	// Create initial job
	completions := int32(3)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-job",
			Namespace:       "default",
			UID:             "job-uid-123",
			ResourceVersion: "1",
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
		},
	}

	clientset := fake.NewSimpleClientset(job)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Update the job
	updatedJob := job.DeepCopy()
	updatedJob.ResourceVersion = "2"
	updatedJob.Status.Succeeded = 2

	_, err := clientset.BatchV1().Jobs("default").Update(context.TODO(), updatedJob, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update job: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeJob)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestJobIngester_OnDelete(t *testing.T) {
	// Create initial job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(job)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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

	// Delete the job
	err := clientset.BatchV1().Jobs("default").Delete(context.TODO(), "test-job", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete job: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeJob {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeJob)
		}
		if event.Name != "test-job" {
			t.Errorf("Name = %v, want %v", event.Name, "test-job")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestJobIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	_ = ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a job - this should not block even though channel is full
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.BatchV1().Jobs("default").Create(context.TODO(), job, metav1.CreateOptions{})
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

func TestJobIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-job",
			Namespace:       "default",
			UID:             "job-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(job)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewJobIngester(factory, IngesterConfig{EventChan: eventChan}, log)
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
	ingester.onUpdate(job, job)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}
