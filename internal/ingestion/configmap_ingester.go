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

// ConfigMapIngester watches ConfigMap resources and sends events.
// SECURITY: This ingester only collects metadata and data KEYS - never values.
// ConfigMap data values are intentionally excluded to protect sensitive configuration.
type ConfigMapIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
	lastVersion     map[string]string // Track resourceVersion to skip duplicate updates
}

// NewConfigMapIngester creates a new ConfigMap ingester.
func NewConfigMapIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *ConfigMapIngester {
	return &ConfigMapIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("configmap-ingester"),
		lastVersion:     make(map[string]string),
	}
}

// RegisterHandlers registers the event handlers with the informer.
func (i *ConfigMapIngester) RegisterHandlers() {
	informer := i.informerFactory.Core().V1().ConfigMaps().Informer()
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    i.onAdd,
		UpdateFunc: i.onUpdate,
		DeleteFunc: i.onDelete,
	})
}

// onAdd handles ConfigMap add events.
func (i *ConfigMapIngester) onAdd(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		i.log.Error(nil, "received non-ConfigMap object in add handler")
		return
	}

	i.log.V(1).Info("ConfigMap added", "name", cm.Name, "namespace", cm.Namespace)

	// Track version for deduplication
	key := cm.Namespace + "/" + cm.Name
	i.lastVersion[key] = cm.ResourceVersion

	i.sendEvent(transport.EventTypeAdded, cm)
}

// onUpdate handles ConfigMap update events.
func (i *ConfigMapIngester) onUpdate(oldObj, newObj interface{}) {
	cm, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		i.log.Error(nil, "received non-ConfigMap object in update handler")
		return
	}

	// Skip if resourceVersion hasn't changed (duplicate event)
	key := cm.Namespace + "/" + cm.Name
	if i.lastVersion[key] == cm.ResourceVersion {
		i.log.V(2).Info("skipping duplicate ConfigMap update", "name", cm.Name, "namespace", cm.Namespace)
		return
	}
	i.lastVersion[key] = cm.ResourceVersion

	i.log.V(1).Info("ConfigMap updated", "name", cm.Name, "namespace", cm.Namespace)
	i.sendEvent(transport.EventTypeUpdated, cm)
}

// onDelete handles ConfigMap delete events.
func (i *ConfigMapIngester) onDelete(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			i.log.Error(nil, "received unexpected object in delete handler")
			return
		}
		cm, ok = tombstone.Obj.(*corev1.ConfigMap)
		if !ok {
			i.log.Error(nil, "tombstone contained non-ConfigMap object")
			return
		}
	}

	i.log.V(1).Info("ConfigMap deleted", "name", cm.Name, "namespace", cm.Namespace)

	// Clean up version tracking
	key := cm.Namespace + "/" + cm.Name
	delete(i.lastVersion, key)

	i.sendDeleteEvent(cm)
}

// sendEvent sends a ConfigMap event to the event channel.
// SECURITY: Only metadata and keys are sent, never data values.
func (i *ConfigMapIngester) sendEvent(eventType transport.EventType, cm *corev1.ConfigMap) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeConfigMap,
		UID:       string(cm.UID),
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Object:    transport.NewConfigMapInfo(cm), // Only metadata and keys, no values
	}

	select {
	case i.config.EventChan <- event:
		// Event sent successfully
	default:
		i.log.Info("event channel full, dropping ConfigMap event",
			"type", eventType,
			"name", cm.Name,
			"namespace", cm.Namespace)
	}
}

// sendDeleteEvent sends a ConfigMap delete event.
func (i *ConfigMapIngester) sendDeleteEvent(cm *corev1.ConfigMap) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeConfigMap,
		UID:       string(cm.UID),
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case i.config.EventChan <- event:
		// Event sent successfully
	default:
		i.log.Info("event channel full, dropping ConfigMap delete event",
			"name", cm.Name,
			"namespace", cm.Namespace)
	}
}
