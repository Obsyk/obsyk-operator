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

// ServiceIngester watches Service resources and sends events to the event channel.
type ServiceIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewServiceIngester creates a new ServiceIngester.
func NewServiceIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *ServiceIngester {
	return &ServiceIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("service-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (s *ServiceIngester) RegisterHandlers() error {
	informer := s.informerFactory.Core().V1().Services().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAdd,
		UpdateFunc: s.onUpdate,
		DeleteFunc: s.onDelete,
	})
	return err
}

// onAdd handles Service addition events.
func (s *ServiceIngester) onAdd(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		s.log.Error(nil, "received non-Service object in add handler")
		return
	}

	s.log.V(2).Info("service added",
		"name", svc.Name,
		"namespace", svc.Namespace,
		"uid", svc.UID)

	s.sendEvent(transport.EventTypeAdded, svc)
}

// onUpdate handles Service update events.
func (s *ServiceIngester) onUpdate(oldObj, newObj interface{}) {
	oldSvc, ok := oldObj.(*corev1.Service)
	if !ok {
		return
	}
	newSvc, ok := newObj.(*corev1.Service)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldSvc.ResourceVersion == newSvc.ResourceVersion {
		return
	}

	s.log.V(2).Info("service updated",
		"name", newSvc.Name,
		"namespace", newSvc.Namespace,
		"uid", newSvc.UID)

	s.sendEvent(transport.EventTypeModified, newSvc)
}

// onDelete handles Service deletion events.
func (s *ServiceIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	svc, ok := obj.(*corev1.Service)
	if !ok {
		s.log.Error(nil, "received non-Service object in delete handler")
		return
	}

	s.log.V(2).Info("service deleted",
		"name", svc.Name,
		"namespace", svc.Namespace,
		"uid", svc.UID)

	s.sendDeleteEvent(svc)
}

// sendEvent sends a Service event to the event channel.
func (s *ServiceIngester) sendEvent(eventType transport.EventType, svc *corev1.Service) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeService,
		UID:       string(svc.UID),
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Object:    transport.NewServiceInfo(svc),
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", svc.Name,
			"namespace", svc.Namespace)
	}
}

// sendDeleteEvent sends a Service delete event (without full object data).
func (s *ServiceIngester) sendDeleteEvent(svc *corev1.Service) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeService,
		UID:       string(svc.UID),
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Object:    nil,
	}

	select {
	case s.config.EventChan <- event:
	default:
		s.log.Info("event channel full, dropping delete event",
			"name", svc.Name,
			"namespace", svc.Namespace)
	}
}
