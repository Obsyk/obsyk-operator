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

// StatefulSetIngester watches StatefulSet resources and sends events to the event channel.
type StatefulSetIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewStatefulSetIngester creates a new StatefulSetIngester.
func NewStatefulSetIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *StatefulSetIngester {
	return &StatefulSetIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("statefulset-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (s *StatefulSetIngester) RegisterHandlers() error {
	informer := s.informerFactory.Apps().V1().StatefulSets().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAdd,
		UpdateFunc: s.onUpdate,
		DeleteFunc: s.onDelete,
	})
	return err
}

// onAdd handles StatefulSet addition events.
func (s *StatefulSetIngester) onAdd(obj interface{}) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		s.log.Error(nil, "received non-StatefulSet object in add handler")
		return
	}

	s.log.V(2).Info("statefulset added",
		"name", sts.Name,
		"namespace", sts.Namespace,
		"uid", sts.UID)

	s.sendEvent(transport.EventTypeAdded, sts)
}

// onUpdate handles StatefulSet update events.
func (s *StatefulSetIngester) onUpdate(oldObj, newObj interface{}) {
	oldSts, ok := oldObj.(*appsv1.StatefulSet)
	if !ok {
		return
	}
	newSts, ok := newObj.(*appsv1.StatefulSet)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldSts.ResourceVersion == newSts.ResourceVersion {
		return
	}

	s.log.V(2).Info("statefulset updated",
		"name", newSts.Name,
		"namespace", newSts.Namespace,
		"uid", newSts.UID)

	s.sendEvent(transport.EventTypeModified, newSts)
}

// onDelete handles StatefulSet deletion events.
func (s *StatefulSetIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		s.log.Error(nil, "received non-StatefulSet object in delete handler")
		return
	}

	s.log.V(2).Info("statefulset deleted",
		"name", sts.Name,
		"namespace", sts.Namespace,
		"uid", sts.UID)

	// For delete events, we only need identifying info, not full object
	s.sendDeleteEvent(sts)
}

// sendEvent sends a StatefulSet event to the event channel.
func (s *StatefulSetIngester) sendEvent(eventType transport.EventType, sts *appsv1.StatefulSet) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeStatefulSet,
		UID:       string(sts.UID),
		Name:      sts.Name,
		Namespace: sts.Namespace,
		Object:    transport.NewStatefulSetInfo(sts),
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", sts.Name,
			"namespace", sts.Namespace)
	}
}

// sendDeleteEvent sends a StatefulSet delete event (without full object data).
func (s *StatefulSetIngester) sendDeleteEvent(sts *appsv1.StatefulSet) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeStatefulSet,
		UID:       string(sts.UID),
		Name:      sts.Name,
		Namespace: sts.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping delete event",
			"name", sts.Name,
			"namespace", sts.Namespace)
	}
}
