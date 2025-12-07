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

// NamespaceIngester watches Namespace resources and sends events to the event channel.
type NamespaceIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewNamespaceIngester creates a new NamespaceIngester.
func NewNamespaceIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *NamespaceIngester {
	return &NamespaceIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("namespace-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (n *NamespaceIngester) RegisterHandlers() {
	informer := n.informerFactory.Core().V1().Namespaces().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd,
		UpdateFunc: n.onUpdate,
		DeleteFunc: n.onDelete,
	})
	if err != nil {
		n.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles Namespace addition events.
func (n *NamespaceIngester) onAdd(obj interface{}) {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		n.log.Error(nil, "received non-Namespace object in add handler")
		return
	}

	n.log.V(2).Info("namespace added",
		"name", ns.Name,
		"uid", ns.UID)

	n.sendEvent(transport.EventTypeAdded, ns)
}

// onUpdate handles Namespace update events.
func (n *NamespaceIngester) onUpdate(oldObj, newObj interface{}) {
	oldNS, ok := oldObj.(*corev1.Namespace)
	if !ok {
		return
	}
	newNS, ok := newObj.(*corev1.Namespace)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldNS.ResourceVersion == newNS.ResourceVersion {
		return
	}

	n.log.V(2).Info("namespace updated",
		"name", newNS.Name,
		"uid", newNS.UID)

	n.sendEvent(transport.EventTypeModified, newNS)
}

// onDelete handles Namespace deletion events.
func (n *NamespaceIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		n.log.Error(nil, "received non-Namespace object in delete handler")
		return
	}

	n.log.V(2).Info("namespace deleted",
		"name", ns.Name,
		"uid", ns.UID)

	n.sendDeleteEvent(ns)
}

// sendEvent sends a Namespace event to the event channel.
func (n *NamespaceIngester) sendEvent(eventType transport.EventType, ns *corev1.Namespace) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeNamespace,
		UID:       string(ns.UID),
		Name:      ns.Name,
		Namespace: "", // Namespaces are cluster-scoped
		Object:    transport.NewNamespaceInfo(ns),
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", ns.Name)
	}
}

// sendDeleteEvent sends a Namespace delete event (without full object data).
func (n *NamespaceIngester) sendDeleteEvent(ns *corev1.Namespace) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeNamespace,
		UID:       string(ns.UID),
		Name:      ns.Name,
		Namespace: "",
		Object:    nil,
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping delete event",
			"name", ns.Name)
	}
}
