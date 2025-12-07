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

// RoleIngester watches Role resources and sends events to the event channel.
type RoleIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewRoleIngester creates a new RoleIngester.
func NewRoleIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *RoleIngester {
	return &RoleIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("role-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (r *RoleIngester) RegisterHandlers() {
	informer := r.informerFactory.Rbac().V1().Roles().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAdd,
		UpdateFunc: r.onUpdate,
		DeleteFunc: r.onDelete,
	})
	if err != nil {
		r.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles Role addition events.
func (r *RoleIngester) onAdd(obj interface{}) {
	role, ok := obj.(*rbacv1.Role)
	if !ok {
		r.log.Error(nil, "received non-Role object in add handler")
		return
	}

	r.log.V(2).Info("role added",
		"name", role.Name,
		"namespace", role.Namespace,
		"uid", role.UID)

	r.sendEvent(transport.EventTypeAdded, role)
}

// onUpdate handles Role update events.
func (r *RoleIngester) onUpdate(oldObj, newObj interface{}) {
	oldRole, ok := oldObj.(*rbacv1.Role)
	if !ok {
		return
	}
	newRole, ok := newObj.(*rbacv1.Role)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldRole.ResourceVersion == newRole.ResourceVersion {
		return
	}

	r.log.V(2).Info("role updated",
		"name", newRole.Name,
		"namespace", newRole.Namespace,
		"uid", newRole.UID)

	r.sendEvent(transport.EventTypeUpdated, newRole)
}

// onDelete handles Role deletion events.
func (r *RoleIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	role, ok := obj.(*rbacv1.Role)
	if !ok {
		r.log.Error(nil, "received non-Role object in delete handler")
		return
	}

	r.log.V(2).Info("role deleted",
		"name", role.Name,
		"namespace", role.Namespace,
		"uid", role.UID)

	// For delete events, we only need identifying info, not full object
	r.sendDeleteEvent(role)
}

// sendEvent sends a Role event to the event channel.
func (r *RoleIngester) sendEvent(eventType transport.EventType, role *rbacv1.Role) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeRole,
		UID:       string(role.UID),
		Name:      role.Name,
		Namespace: role.Namespace,
		Object:    transport.NewRoleInfo(role),
	}

	select {
	case r.config.EventChan <- event:
	default:
		r.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", role.Name,
			"namespace", role.Namespace)
	}
}

// sendDeleteEvent sends a Role delete event (without full object data).
func (r *RoleIngester) sendDeleteEvent(role *rbacv1.Role) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeRole,
		UID:       string(role.UID),
		Name:      role.Name,
		Namespace: role.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case r.config.EventChan <- event:
	default:
		r.log.Error(nil, "event channel full, dropping delete event",
			"name", role.Name,
			"namespace", role.Namespace)
	}
}
