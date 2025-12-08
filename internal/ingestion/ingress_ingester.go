// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// IngressIngester watches Ingress resources and sends events to the event channel.
type IngressIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewIngressIngester creates a new IngressIngester.
func NewIngressIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *IngressIngester {
	return &IngressIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("ingress-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (i *IngressIngester) RegisterHandlers() error {
	informer := i.informerFactory.Networking().V1().Ingresses().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    i.onAdd,
		UpdateFunc: i.onUpdate,
		DeleteFunc: i.onDelete,
	})
	return err
}

// onAdd handles Ingress addition events.
func (i *IngressIngester) onAdd(obj interface{}) {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		i.log.Error(nil, "received non-Ingress object in add handler")
		return
	}

	i.log.V(2).Info("ingress added",
		"name", ing.Name,
		"namespace", ing.Namespace,
		"uid", ing.UID)

	i.sendEvent(transport.EventTypeAdded, ing)
}

// onUpdate handles Ingress update events.
func (i *IngressIngester) onUpdate(oldObj, newObj interface{}) {
	oldIng, ok := oldObj.(*networkingv1.Ingress)
	if !ok {
		return
	}
	newIng, ok := newObj.(*networkingv1.Ingress)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldIng.ResourceVersion == newIng.ResourceVersion {
		return
	}

	i.log.V(2).Info("ingress updated",
		"name", newIng.Name,
		"namespace", newIng.Namespace,
		"uid", newIng.UID)

	i.sendEvent(transport.EventTypeModified, newIng)
}

// onDelete handles Ingress deletion events.
func (i *IngressIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		i.log.Error(nil, "received non-Ingress object in delete handler")
		return
	}

	i.log.V(2).Info("ingress deleted",
		"name", ing.Name,
		"namespace", ing.Namespace,
		"uid", ing.UID)

	// For delete events, we only need identifying info, not full object
	i.sendDeleteEvent(ing)
}

// sendEvent sends an Ingress event to the event channel.
func (i *IngressIngester) sendEvent(eventType transport.EventType, ing *networkingv1.Ingress) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeIngress,
		UID:       string(ing.UID),
		Name:      ing.Name,
		Namespace: ing.Namespace,
		Object:    transport.NewIngressInfo(ing),
	}

	select {
	case i.config.EventChan <- event:
	default:
		i.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", ing.Name,
			"namespace", ing.Namespace)
	}
}

// sendDeleteEvent sends an Ingress delete event (without full object data).
func (i *IngressIngester) sendDeleteEvent(ing *networkingv1.Ingress) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeIngress,
		UID:       string(ing.UID),
		Name:      ing.Name,
		Namespace: ing.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case i.config.EventChan <- event:
	default:
		i.log.Info("event channel full, dropping delete event",
			"name", ing.Name,
			"namespace", ing.Namespace)
	}
}
