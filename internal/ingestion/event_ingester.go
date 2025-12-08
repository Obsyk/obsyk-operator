// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// EventIngester watches Kubernetes Event resources and sends events to the event channel.
type EventIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewEventIngester creates a new EventIngester.
func NewEventIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *EventIngester {
	return &EventIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("event-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (e *EventIngester) RegisterHandlers() error {
	informer := e.informerFactory.Core().V1().Events().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    e.onAdd,
		UpdateFunc: e.onUpdate,
		DeleteFunc: e.onDelete,
	})
	return err
}

// onAdd handles Event addition events.
func (e *EventIngester) onAdd(obj interface{}) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		e.log.Error(nil, "received non-Event object in add handler")
		return
	}

	e.log.V(2).Info("k8s event added",
		"name", event.Name,
		"namespace", event.Namespace,
		"type", event.Type,
		"reason", event.Reason)

	e.sendEvent(transport.EventTypeAdded, event)
}

// onUpdate handles Event update events.
func (e *EventIngester) onUpdate(oldObj, newObj interface{}) {
	oldEvent, ok := oldObj.(*corev1.Event)
	if !ok {
		return
	}
	newEvent, ok := newObj.(*corev1.Event)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldEvent.ResourceVersion == newEvent.ResourceVersion {
		return
	}

	e.log.V(2).Info("k8s event updated",
		"name", newEvent.Name,
		"namespace", newEvent.Namespace,
		"type", newEvent.Type,
		"reason", newEvent.Reason,
		"count", newEvent.Count)

	e.sendEvent(transport.EventTypeModified, newEvent)
}

// onDelete handles Event deletion events.
func (e *EventIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	event, ok := obj.(*corev1.Event)
	if !ok {
		e.log.Error(nil, "received non-Event object in delete handler")
		return
	}

	e.log.V(2).Info("k8s event deleted",
		"name", event.Name,
		"namespace", event.Namespace)

	// For delete events, we only need identifying info, not full object
	e.sendDeleteEvent(event)
}

// sendEvent sends a Kubernetes Event to the event channel.
func (e *EventIngester) sendEvent(eventType transport.EventType, event *corev1.Event) {
	resourceEvent := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeEvent,
		UID:       string(event.UID),
		Name:      event.Name,
		Namespace: event.Namespace,
		Object:    transport.NewEventInfo(event),
	}

	select {
	case e.config.EventChan <- resourceEvent:
	default:
		e.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", event.Name,
			"namespace", event.Namespace)
	}
}

// sendDeleteEvent sends a Kubernetes Event delete event (without full object data).
func (e *EventIngester) sendDeleteEvent(event *corev1.Event) {
	resourceEvent := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeEvent,
		UID:       string(event.UID),
		Name:      event.Name,
		Namespace: event.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case e.config.EventChan <- resourceEvent:
	default:
		e.log.Info("event channel full, dropping delete event",
			"name", event.Name,
			"namespace", event.Namespace)
	}
}
