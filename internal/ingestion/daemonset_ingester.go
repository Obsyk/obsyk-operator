// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// DaemonSetIngester watches DaemonSet resources and sends events to the event channel.
type DaemonSetIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewDaemonSetIngester creates a new DaemonSetIngester.
func NewDaemonSetIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *DaemonSetIngester {
	return &DaemonSetIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("daemonset-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (d *DaemonSetIngester) RegisterHandlers() error {
	informer := d.informerFactory.Apps().V1().DaemonSets().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    d.onAdd,
		UpdateFunc: d.onUpdate,
		DeleteFunc: d.onDelete,
	})
	return err
}

// onAdd handles DaemonSet addition events.
func (d *DaemonSetIngester) onAdd(obj interface{}) {
	ds, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		d.log.Error(nil, "received non-DaemonSet object in add handler")
		return
	}

	d.log.V(2).Info("daemonset added",
		"name", ds.Name,
		"namespace", ds.Namespace,
		"uid", ds.UID)

	d.sendEvent(transport.EventTypeAdded, ds)
}

// onUpdate handles DaemonSet update events.
func (d *DaemonSetIngester) onUpdate(oldObj, newObj interface{}) {
	oldDs, ok := oldObj.(*appsv1.DaemonSet)
	if !ok {
		return
	}
	newDs, ok := newObj.(*appsv1.DaemonSet)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldDs.ResourceVersion == newDs.ResourceVersion {
		return
	}

	d.log.V(2).Info("daemonset updated",
		"name", newDs.Name,
		"namespace", newDs.Namespace,
		"uid", newDs.UID)

	d.sendEvent(transport.EventTypeModified, newDs)
}

// onDelete handles DaemonSet deletion events.
func (d *DaemonSetIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	ds, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		d.log.Error(nil, "received non-DaemonSet object in delete handler")
		return
	}

	d.log.V(2).Info("daemonset deleted",
		"name", ds.Name,
		"namespace", ds.Namespace,
		"uid", ds.UID)

	// For delete events, we only need identifying info, not full object
	d.sendDeleteEvent(ds)
}

// sendEvent sends a DaemonSet event to the event channel.
func (d *DaemonSetIngester) sendEvent(eventType transport.EventType, ds *appsv1.DaemonSet) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeDaemonSet,
		UID:       string(ds.UID),
		Name:      ds.Name,
		Namespace: ds.Namespace,
		Object:    transport.NewDaemonSetInfo(ds),
	}

	select {
	case d.config.EventChan <- event:
	default:
		d.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", ds.Name,
			"namespace", ds.Namespace)
	}
}

// sendDeleteEvent sends a DaemonSet delete event (without full object data).
func (d *DaemonSetIngester) sendDeleteEvent(ds *appsv1.DaemonSet) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeDaemonSet,
		UID:       string(ds.UID),
		Name:      ds.Name,
		Namespace: ds.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case d.config.EventChan <- event:
	default:
		d.log.Error(nil, "event channel full, dropping delete event",
			"name", ds.Name,
			"namespace", ds.Namespace)
	}
}
