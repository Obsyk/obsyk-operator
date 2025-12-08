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

// ServiceAccountIngester watches ServiceAccount resources and sends events to the event channel.
type ServiceAccountIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewServiceAccountIngester creates a new ServiceAccountIngester.
func NewServiceAccountIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *ServiceAccountIngester {
	return &ServiceAccountIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("serviceaccount-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (s *ServiceAccountIngester) RegisterHandlers() error {
	informer := s.informerFactory.Core().V1().ServiceAccounts().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAdd,
		UpdateFunc: s.onUpdate,
		DeleteFunc: s.onDelete,
	})
	return err
}

// onAdd handles ServiceAccount addition events.
func (s *ServiceAccountIngester) onAdd(obj interface{}) {
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		s.log.Error(nil, "received non-ServiceAccount object in add handler")
		return
	}

	s.log.V(2).Info("serviceaccount added",
		"name", sa.Name,
		"namespace", sa.Namespace,
		"uid", sa.UID)

	s.sendEvent(transport.EventTypeAdded, sa)
}

// onUpdate handles ServiceAccount update events.
func (s *ServiceAccountIngester) onUpdate(oldObj, newObj interface{}) {
	oldSA, ok := oldObj.(*corev1.ServiceAccount)
	if !ok {
		return
	}
	newSA, ok := newObj.(*corev1.ServiceAccount)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldSA.ResourceVersion == newSA.ResourceVersion {
		return
	}

	s.log.V(2).Info("serviceaccount updated",
		"name", newSA.Name,
		"namespace", newSA.Namespace,
		"uid", newSA.UID)

	s.sendEvent(transport.EventTypeModified, newSA)
}

// onDelete handles ServiceAccount deletion events.
func (s *ServiceAccountIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		s.log.Error(nil, "received non-ServiceAccount object in delete handler")
		return
	}

	s.log.V(2).Info("serviceaccount deleted",
		"name", sa.Name,
		"namespace", sa.Namespace,
		"uid", sa.UID)

	// For delete events, we only need identifying info, not full object
	s.sendDeleteEvent(sa)
}

// sendEvent sends a ServiceAccount event to the event channel.
func (s *ServiceAccountIngester) sendEvent(eventType transport.EventType, sa *corev1.ServiceAccount) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeServiceAccount,
		UID:       string(sa.UID),
		Name:      sa.Name,
		Namespace: sa.Namespace,
		Object:    transport.NewServiceAccountInfo(sa),
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", sa.Name,
			"namespace", sa.Namespace)
	}
}

// sendDeleteEvent sends a ServiceAccount delete event (without full object data).
func (s *ServiceAccountIngester) sendDeleteEvent(sa *corev1.ServiceAccount) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeServiceAccount,
		UID:       string(sa.UID),
		Name:      sa.Name,
		Namespace: sa.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping delete event",
			"name", sa.Name,
			"namespace", sa.Namespace)
	}
}
