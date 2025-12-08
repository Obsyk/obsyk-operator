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

// NetworkPolicyIngester watches NetworkPolicy resources and sends events to the event channel.
type NetworkPolicyIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewNetworkPolicyIngester creates a new NetworkPolicyIngester.
func NewNetworkPolicyIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *NetworkPolicyIngester {
	return &NetworkPolicyIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("networkpolicy-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (n *NetworkPolicyIngester) RegisterHandlers() error {
	informer := n.informerFactory.Networking().V1().NetworkPolicies().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd,
		UpdateFunc: n.onUpdate,
		DeleteFunc: n.onDelete,
	})
	return err
}

// onAdd handles NetworkPolicy addition events.
func (n *NetworkPolicyIngester) onAdd(obj interface{}) {
	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		n.log.Error(nil, "received non-NetworkPolicy object in add handler")
		return
	}

	n.log.V(2).Info("networkpolicy added",
		"name", np.Name,
		"namespace", np.Namespace,
		"uid", np.UID)

	n.sendEvent(transport.EventTypeAdded, np)
}

// onUpdate handles NetworkPolicy update events.
func (n *NetworkPolicyIngester) onUpdate(oldObj, newObj interface{}) {
	oldNP, ok := oldObj.(*networkingv1.NetworkPolicy)
	if !ok {
		return
	}
	newNP, ok := newObj.(*networkingv1.NetworkPolicy)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldNP.ResourceVersion == newNP.ResourceVersion {
		return
	}

	n.log.V(2).Info("networkpolicy updated",
		"name", newNP.Name,
		"namespace", newNP.Namespace,
		"uid", newNP.UID)

	n.sendEvent(transport.EventTypeModified, newNP)
}

// onDelete handles NetworkPolicy deletion events.
func (n *NetworkPolicyIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		n.log.Error(nil, "received non-NetworkPolicy object in delete handler")
		return
	}

	n.log.V(2).Info("networkpolicy deleted",
		"name", np.Name,
		"namespace", np.Namespace,
		"uid", np.UID)

	// For delete events, we only need identifying info, not full object
	n.sendDeleteEvent(np)
}

// sendEvent sends a NetworkPolicy event to the event channel.
func (n *NetworkPolicyIngester) sendEvent(eventType transport.EventType, np *networkingv1.NetworkPolicy) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeNetworkPolicy,
		UID:       string(np.UID),
		Name:      np.Name,
		Namespace: np.Namespace,
		Object:    transport.NewNetworkPolicyInfo(np),
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", np.Name,
			"namespace", np.Namespace)
	}
}

// sendDeleteEvent sends a NetworkPolicy delete event (without full object data).
func (n *NetworkPolicyIngester) sendDeleteEvent(np *networkingv1.NetworkPolicy) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeNetworkPolicy,
		UID:       string(np.UID),
		Name:      np.Name,
		Namespace: np.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping delete event",
			"name", np.Name,
			"namespace", np.Namespace)
	}
}
