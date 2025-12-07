// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// JobIngester watches Job resources and sends events to the event channel.
type JobIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewJobIngester creates a new JobIngester.
func NewJobIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *JobIngester {
	return &JobIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("job-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (j *JobIngester) RegisterHandlers() {
	informer := j.informerFactory.Batch().V1().Jobs().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    j.onAdd,
		UpdateFunc: j.onUpdate,
		DeleteFunc: j.onDelete,
	})
	if err != nil {
		j.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles Job addition events.
func (j *JobIngester) onAdd(obj interface{}) {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		j.log.Error(nil, "received non-Job object in add handler")
		return
	}

	j.log.V(2).Info("job added",
		"name", job.Name,
		"namespace", job.Namespace,
		"uid", job.UID)

	j.sendEvent(transport.EventTypeAdded, job)
}

// onUpdate handles Job update events.
func (j *JobIngester) onUpdate(oldObj, newObj interface{}) {
	oldJob, ok := oldObj.(*batchv1.Job)
	if !ok {
		return
	}
	newJob, ok := newObj.(*batchv1.Job)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldJob.ResourceVersion == newJob.ResourceVersion {
		return
	}

	j.log.V(2).Info("job updated",
		"name", newJob.Name,
		"namespace", newJob.Namespace,
		"uid", newJob.UID)

	j.sendEvent(transport.EventTypeModified, newJob)
}

// onDelete handles Job deletion events.
func (j *JobIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	job, ok := obj.(*batchv1.Job)
	if !ok {
		j.log.Error(nil, "received non-Job object in delete handler")
		return
	}

	j.log.V(2).Info("job deleted",
		"name", job.Name,
		"namespace", job.Namespace,
		"uid", job.UID)

	// For delete events, we only need identifying info, not full object
	j.sendDeleteEvent(job)
}

// sendEvent sends a Job event to the event channel.
func (j *JobIngester) sendEvent(eventType transport.EventType, job *batchv1.Job) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeJob,
		UID:       string(job.UID),
		Name:      job.Name,
		Namespace: job.Namespace,
		Object:    transport.NewJobInfo(job),
	}

	select {
	case j.config.EventChan <- event:
	default:
		j.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", job.Name,
			"namespace", job.Namespace)
	}
}

// sendDeleteEvent sends a Job delete event (without full object data).
func (j *JobIngester) sendDeleteEvent(job *batchv1.Job) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeJob,
		UID:       string(job.UID),
		Name:      job.Name,
		Namespace: job.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case j.config.EventChan <- event:
	default:
		j.log.Error(nil, "event channel full, dropping delete event",
			"name", job.Name,
			"namespace", job.Namespace)
	}
}
