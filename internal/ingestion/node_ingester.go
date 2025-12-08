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

// NodeIngester watches Node resources and sends events to the event channel.
type NodeIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewNodeIngester creates a new NodeIngester.
func NewNodeIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *NodeIngester {
	return &NodeIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("node-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (n *NodeIngester) RegisterHandlers() error {
	informer := n.informerFactory.Core().V1().Nodes().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd,
		UpdateFunc: n.onUpdate,
		DeleteFunc: n.onDelete,
	})
	return err
}

// onAdd handles Node addition events.
func (n *NodeIngester) onAdd(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		n.log.Error(nil, "received non-Node object in add handler")
		return
	}

	n.log.V(2).Info("node added",
		"name", node.Name,
		"uid", node.UID)

	n.sendEvent(transport.EventTypeAdded, node)
}

// onUpdate handles Node update events.
func (n *NodeIngester) onUpdate(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}
	newNode, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldNode.ResourceVersion == newNode.ResourceVersion {
		return
	}

	n.log.V(2).Info("node updated",
		"name", newNode.Name,
		"uid", newNode.UID)

	n.sendEvent(transport.EventTypeModified, newNode)
}

// onDelete handles Node deletion events.
func (n *NodeIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	node, ok := obj.(*corev1.Node)
	if !ok {
		n.log.Error(nil, "received non-Node object in delete handler")
		return
	}

	n.log.V(2).Info("node deleted",
		"name", node.Name,
		"uid", node.UID)

	// For delete events, we only need identifying info, not full object
	n.sendDeleteEvent(node)
}

// sendEvent sends a Node event to the event channel.
func (n *NodeIngester) sendEvent(eventType transport.EventType, node *corev1.Node) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeNode,
		UID:       string(node.UID),
		Name:      node.Name,
		Namespace: "", // Nodes are cluster-scoped
		Object:    transport.NewNodeInfo(node),
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", node.Name)
	}
}

// sendDeleteEvent sends a Node delete event (without full object data).
func (n *NodeIngester) sendDeleteEvent(node *corev1.Node) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeNode,
		UID:       string(node.UID),
		Name:      node.Name,
		Namespace: "",  // Nodes are cluster-scoped
		Object:    nil, // No object data for deletes
	}

	select {
	case n.config.EventChan <- event:
	default:
		n.log.Error(nil, "event channel full, dropping delete event",
			"name", node.Name)
	}
}
