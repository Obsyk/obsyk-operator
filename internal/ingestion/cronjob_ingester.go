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

// CronJobIngester watches CronJob resources and sends events to the event channel.
type CronJobIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewCronJobIngester creates a new CronJobIngester.
func NewCronJobIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *CronJobIngester {
	return &CronJobIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("cronjob-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (c *CronJobIngester) RegisterHandlers() {
	informer := c.informerFactory.Batch().V1().CronJobs().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	})
	if err != nil {
		c.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles CronJob addition events.
func (c *CronJobIngester) onAdd(obj interface{}) {
	cj, ok := obj.(*batchv1.CronJob)
	if !ok {
		c.log.Error(nil, "received non-CronJob object in add handler")
		return
	}

	c.log.V(2).Info("cronjob added",
		"name", cj.Name,
		"namespace", cj.Namespace,
		"uid", cj.UID)

	c.sendEvent(transport.EventTypeAdded, cj)
}

// onUpdate handles CronJob update events.
func (c *CronJobIngester) onUpdate(oldObj, newObj interface{}) {
	oldCJ, ok := oldObj.(*batchv1.CronJob)
	if !ok {
		return
	}
	newCJ, ok := newObj.(*batchv1.CronJob)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldCJ.ResourceVersion == newCJ.ResourceVersion {
		return
	}

	c.log.V(2).Info("cronjob updated",
		"name", newCJ.Name,
		"namespace", newCJ.Namespace,
		"uid", newCJ.UID)

	c.sendEvent(transport.EventTypeUpdated, newCJ)
}

// onDelete handles CronJob deletion events.
func (c *CronJobIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	cj, ok := obj.(*batchv1.CronJob)
	if !ok {
		c.log.Error(nil, "received non-CronJob object in delete handler")
		return
	}

	c.log.V(2).Info("cronjob deleted",
		"name", cj.Name,
		"namespace", cj.Namespace,
		"uid", cj.UID)

	// For delete events, we only need identifying info, not full object
	c.sendDeleteEvent(cj)
}

// sendEvent sends a CronJob event to the event channel.
func (c *CronJobIngester) sendEvent(eventType transport.EventType, cj *batchv1.CronJob) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeCronJob,
		UID:       string(cj.UID),
		Name:      cj.Name,
		Namespace: cj.Namespace,
		Object:    transport.NewCronJobInfo(cj),
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", cj.Name,
			"namespace", cj.Namespace)
	}
}

// sendDeleteEvent sends a CronJob delete event (without full object data).
func (c *CronJobIngester) sendDeleteEvent(cj *batchv1.CronJob) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeCronJob,
		UID:       string(cj.UID),
		Name:      cj.Name,
		Namespace: cj.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping delete event",
			"name", cj.Name,
			"namespace", cj.Namespace)
	}
}
