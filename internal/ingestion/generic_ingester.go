// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// KubeObject is a constraint for Kubernetes objects that have ObjectMeta.
// All Kubernetes resources implement runtime.Object and have metadata.
type KubeObject interface {
	runtime.Object
	metav1.Object
}

// ResourceConfig holds the configuration for a specific resource type.
// This allows the generic ingester to handle different resource types
// with type-safe callbacks.
type ResourceConfig[T KubeObject] struct {
	// Name is the human-readable name for logging (e.g., "pod", "deployment")
	Name string

	// ResourceType is the transport resource type for events
	ResourceType transport.ResourceType

	// GetInformer returns the cache.SharedIndexInformer for this resource type.
	// This is called during RegisterHandlers to get the appropriate informer.
	GetInformer func() cache.SharedIndexInformer

	// ToInfo converts a Kubernetes object to its transport info type.
	// Returns nil if conversion fails.
	ToInfo func(T) interface{}

	// IsNamespaced indicates whether this resource is namespace-scoped.
	// If false, the Namespace field will be empty in events.
	IsNamespaced bool
}

// GenericIngester watches Kubernetes resources of type T and sends events to the event channel.
// It provides a type-safe, generic implementation that eliminates code duplication
// across the 20 different resource ingesters.
type GenericIngester[T KubeObject] struct {
	config      IngesterConfig
	resourceCfg ResourceConfig[T]
	log         logr.Logger
}

// NewGenericIngester creates a new generic ingester for the specified resource type.
func NewGenericIngester[T KubeObject](
	resourceCfg ResourceConfig[T],
	ingesterCfg IngesterConfig,
	log logr.Logger,
) *GenericIngester[T] {
	return &GenericIngester[T]{
		config:      ingesterCfg,
		resourceCfg: resourceCfg,
		log:         log.WithName(resourceCfg.Name + "-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (g *GenericIngester[T]) RegisterHandlers() error {
	informer := g.resourceCfg.GetInformer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    g.onAdd,
		UpdateFunc: g.onUpdate,
		DeleteFunc: g.onDelete,
	})
	return err
}

// onAdd handles resource addition events.
func (g *GenericIngester[T]) onAdd(obj interface{}) {
	resource, ok := obj.(T)
	if !ok {
		g.log.Error(nil, "received unexpected object type in add handler",
			"expectedType", g.resourceCfg.Name)
		return
	}

	if g.resourceCfg.IsNamespaced {
		g.log.V(2).Info(g.resourceCfg.Name+" added",
			"name", resource.GetName(),
			"namespace", resource.GetNamespace(),
			"uid", resource.GetUID())
	} else {
		g.log.V(2).Info(g.resourceCfg.Name+" added",
			"name", resource.GetName(),
			"uid", resource.GetUID())
	}

	g.sendEvent(transport.EventTypeAdded, resource)
}

// onUpdate handles resource update events.
func (g *GenericIngester[T]) onUpdate(oldObj, newObj interface{}) {
	oldResource, ok := oldObj.(T)
	if !ok {
		return
	}
	newResource, ok := newObj.(T)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldResource.GetResourceVersion() == newResource.GetResourceVersion() {
		return
	}

	if g.resourceCfg.IsNamespaced {
		g.log.V(2).Info(g.resourceCfg.Name+" updated",
			"name", newResource.GetName(),
			"namespace", newResource.GetNamespace(),
			"uid", newResource.GetUID())
	} else {
		g.log.V(2).Info(g.resourceCfg.Name+" updated",
			"name", newResource.GetName(),
			"uid", newResource.GetUID())
	}

	g.sendEvent(transport.EventTypeModified, newResource)
}

// onDelete handles resource deletion events.
func (g *GenericIngester[T]) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	resource, ok := obj.(T)
	if !ok {
		g.log.Error(nil, "received unexpected object type in delete handler",
			"expectedType", g.resourceCfg.Name)
		return
	}

	if g.resourceCfg.IsNamespaced {
		g.log.V(2).Info(g.resourceCfg.Name+" deleted",
			"name", resource.GetName(),
			"namespace", resource.GetNamespace(),
			"uid", resource.GetUID())
	} else {
		g.log.V(2).Info(g.resourceCfg.Name+" deleted",
			"name", resource.GetName(),
			"uid", resource.GetUID())
	}

	// For delete events, we only need identifying info, not full object
	g.sendDeleteEvent(resource)
}

// sendEvent sends a resource event to the event channel.
func (g *GenericIngester[T]) sendEvent(eventType transport.EventType, resource T) {
	namespace := ""
	if g.resourceCfg.IsNamespaced {
		namespace = resource.GetNamespace()
	}

	event := ResourceEvent{
		Type:      eventType,
		Kind:      g.resourceCfg.ResourceType,
		UID:       string(resource.GetUID()),
		Name:      resource.GetName(),
		Namespace: namespace,
		Object:    g.resourceCfg.ToInfo(resource),
	}

	select {
	case g.config.EventChan <- event:
	default:
		if g.resourceCfg.IsNamespaced {
			g.log.Info("event channel full, dropping event",
				"type", eventType,
				"name", resource.GetName(),
				"namespace", resource.GetNamespace())
		} else {
			g.log.Info("event channel full, dropping event",
				"type", eventType,
				"name", resource.GetName())
		}
	}
}

// sendDeleteEvent sends a resource delete event (without full object data).
func (g *GenericIngester[T]) sendDeleteEvent(resource T) {
	namespace := ""
	if g.resourceCfg.IsNamespaced {
		namespace = resource.GetNamespace()
	}

	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      g.resourceCfg.ResourceType,
		UID:       string(resource.GetUID()),
		Name:      resource.GetName(),
		Namespace: namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case g.config.EventChan <- event:
	default:
		if g.resourceCfg.IsNamespaced {
			g.log.Info("event channel full, dropping delete event",
				"name", resource.GetName(),
				"namespace", resource.GetNamespace())
		} else {
			g.log.Info("event channel full, dropping delete event",
				"name", resource.GetName())
		}
	}
}
