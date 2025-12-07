// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// ClusterRoleBindingIngester watches ClusterRoleBinding resources and sends events to the event channel.
type ClusterRoleBindingIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewClusterRoleBindingIngester creates a new ClusterRoleBindingIngester.
func NewClusterRoleBindingIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *ClusterRoleBindingIngester {
	return &ClusterRoleBindingIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("clusterrolebinding-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (c *ClusterRoleBindingIngester) RegisterHandlers() {
	informer := c.informerFactory.Rbac().V1().ClusterRoleBindings().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	})
	if err != nil {
		c.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles ClusterRoleBinding addition events.
func (c *ClusterRoleBindingIngester) onAdd(obj interface{}) {
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		c.log.Error(nil, "received non-ClusterRoleBinding object in add handler")
		return
	}

	c.log.V(2).Info("clusterrolebinding added",
		"name", crb.Name,
		"uid", crb.UID)

	c.sendEvent(transport.EventTypeAdded, crb)
}

// onUpdate handles ClusterRoleBinding update events.
func (c *ClusterRoleBindingIngester) onUpdate(oldObj, newObj interface{}) {
	oldCRB, ok := oldObj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return
	}
	newCRB, ok := newObj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldCRB.ResourceVersion == newCRB.ResourceVersion {
		return
	}

	c.log.V(2).Info("clusterrolebinding updated",
		"name", newCRB.Name,
		"uid", newCRB.UID)

	c.sendEvent(transport.EventTypeUpdated, newCRB)
}

// onDelete handles ClusterRoleBinding deletion events.
func (c *ClusterRoleBindingIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		c.log.Error(nil, "received non-ClusterRoleBinding object in delete handler")
		return
	}

	c.log.V(2).Info("clusterrolebinding deleted",
		"name", crb.Name,
		"uid", crb.UID)

	// For delete events, we only need identifying info, not full object
	c.sendDeleteEvent(crb)
}

// sendEvent sends a ClusterRoleBinding event to the event channel.
func (c *ClusterRoleBindingIngester) sendEvent(eventType transport.EventType, crb *rbacv1.ClusterRoleBinding) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeClusterRoleBinding,
		UID:       string(crb.UID),
		Name:      crb.Name,
		Namespace: "", // ClusterRoleBinding is cluster-scoped
		Object:    transport.NewClusterRoleBindingInfo(crb),
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", crb.Name)
	}
}

// sendDeleteEvent sends a ClusterRoleBinding delete event (without full object data).
func (c *ClusterRoleBindingIngester) sendDeleteEvent(crb *rbacv1.ClusterRoleBinding) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeClusterRoleBinding,
		UID:       string(crb.UID),
		Name:      crb.Name,
		Namespace: "",  // ClusterRoleBinding is cluster-scoped
		Object:    nil, // No object data for deletes
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping delete event",
			"name", crb.Name)
	}
}
