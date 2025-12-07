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

// RoleBindingIngester watches RoleBinding resources and sends events to the event channel.
type RoleBindingIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewRoleBindingIngester creates a new RoleBindingIngester.
func NewRoleBindingIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *RoleBindingIngester {
	return &RoleBindingIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("rolebinding-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (r *RoleBindingIngester) RegisterHandlers() {
	informer := r.informerFactory.Rbac().V1().RoleBindings().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAdd,
		UpdateFunc: r.onUpdate,
		DeleteFunc: r.onDelete,
	})
	if err != nil {
		r.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles RoleBinding addition events.
func (r *RoleBindingIngester) onAdd(obj interface{}) {
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		r.log.Error(nil, "received non-RoleBinding object in add handler")
		return
	}

	r.log.V(2).Info("rolebinding added",
		"name", rb.Name,
		"namespace", rb.Namespace,
		"uid", rb.UID)

	r.sendEvent(transport.EventTypeAdded, rb)
}

// onUpdate handles RoleBinding update events.
func (r *RoleBindingIngester) onUpdate(oldObj, newObj interface{}) {
	oldRB, ok := oldObj.(*rbacv1.RoleBinding)
	if !ok {
		return
	}
	newRB, ok := newObj.(*rbacv1.RoleBinding)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldRB.ResourceVersion == newRB.ResourceVersion {
		return
	}

	r.log.V(2).Info("rolebinding updated",
		"name", newRB.Name,
		"namespace", newRB.Namespace,
		"uid", newRB.UID)

	r.sendEvent(transport.EventTypeUpdated, newRB)
}

// onDelete handles RoleBinding deletion events.
func (r *RoleBindingIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		r.log.Error(nil, "received non-RoleBinding object in delete handler")
		return
	}

	r.log.V(2).Info("rolebinding deleted",
		"name", rb.Name,
		"namespace", rb.Namespace,
		"uid", rb.UID)

	// For delete events, we only need identifying info, not full object
	r.sendDeleteEvent(rb)
}

// sendEvent sends a RoleBinding event to the event channel.
func (r *RoleBindingIngester) sendEvent(eventType transport.EventType, rb *rbacv1.RoleBinding) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeRoleBinding,
		UID:       string(rb.UID),
		Name:      rb.Name,
		Namespace: rb.Namespace,
		Object:    transport.NewRoleBindingInfo(rb),
	}

	select {
	case r.config.EventChan <- event:
	default:
		r.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", rb.Name,
			"namespace", rb.Namespace)
	}
}

// sendDeleteEvent sends a RoleBinding delete event (without full object data).
func (r *RoleBindingIngester) sendDeleteEvent(rb *rbacv1.RoleBinding) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeRoleBinding,
		UID:       string(rb.UID),
		Name:      rb.Name,
		Namespace: rb.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case r.config.EventChan <- event:
	default:
		r.log.Error(nil, "event channel full, dropping delete event",
			"name", rb.Name,
			"namespace", rb.Namespace)
	}
}
