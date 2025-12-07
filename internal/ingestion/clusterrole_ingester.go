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

// ClusterRoleIngester watches ClusterRole resources and sends events to the event channel.
type ClusterRoleIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewClusterRoleIngester creates a new ClusterRoleIngester.
func NewClusterRoleIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *ClusterRoleIngester {
	return &ClusterRoleIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("clusterrole-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (c *ClusterRoleIngester) RegisterHandlers() {
	informer := c.informerFactory.Rbac().V1().ClusterRoles().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	})
	if err != nil {
		c.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles ClusterRole addition events.
func (c *ClusterRoleIngester) onAdd(obj interface{}) {
	clusterRole, ok := obj.(*rbacv1.ClusterRole)
	if !ok {
		c.log.Error(nil, "received non-ClusterRole object in add handler")
		return
	}

	c.log.V(2).Info("clusterrole added",
		"name", clusterRole.Name,
		"uid", clusterRole.UID)

	c.sendEvent(transport.EventTypeAdded, clusterRole)
}

// onUpdate handles ClusterRole update events.
func (c *ClusterRoleIngester) onUpdate(oldObj, newObj interface{}) {
	oldClusterRole, ok := oldObj.(*rbacv1.ClusterRole)
	if !ok {
		return
	}
	newClusterRole, ok := newObj.(*rbacv1.ClusterRole)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldClusterRole.ResourceVersion == newClusterRole.ResourceVersion {
		return
	}

	c.log.V(2).Info("clusterrole updated",
		"name", newClusterRole.Name,
		"uid", newClusterRole.UID)

	c.sendEvent(transport.EventTypeUpdated, newClusterRole)
}

// onDelete handles ClusterRole deletion events.
func (c *ClusterRoleIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	clusterRole, ok := obj.(*rbacv1.ClusterRole)
	if !ok {
		c.log.Error(nil, "received non-ClusterRole object in delete handler")
		return
	}

	c.log.V(2).Info("clusterrole deleted",
		"name", clusterRole.Name,
		"uid", clusterRole.UID)

	// For delete events, we only need identifying info, not full object
	c.sendDeleteEvent(clusterRole)
}

// sendEvent sends a ClusterRole event to the event channel.
func (c *ClusterRoleIngester) sendEvent(eventType transport.EventType, clusterRole *rbacv1.ClusterRole) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeClusterRole,
		UID:       string(clusterRole.UID),
		Name:      clusterRole.Name,
		Namespace: "", // ClusterRole is cluster-scoped
		Object:    transport.NewClusterRoleInfo(clusterRole),
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", clusterRole.Name)
	}
}

// sendDeleteEvent sends a ClusterRole delete event (without full object data).
func (c *ClusterRoleIngester) sendDeleteEvent(clusterRole *rbacv1.ClusterRole) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeClusterRole,
		UID:       string(clusterRole.UID),
		Name:      clusterRole.Name,
		Namespace: "",  // ClusterRole is cluster-scoped
		Object:    nil, // No object data for deletes
	}

	select {
	case c.config.EventChan <- event:
	default:
		c.log.Error(nil, "event channel full, dropping delete event",
			"name", clusterRole.Name)
	}
}
