// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"sync"

	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// SecretIngester watches Secret resources and sends events.
//
// SECURITY WARNING: This ingester ONLY collects metadata and data KEYS.
// Secret data VALUES are NEVER collected, transmitted, or logged.
// This is a critical security requirement - secret data must never leave the cluster.
type SecretIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
	lastVersion     map[string]string // Track resourceVersion to skip duplicate updates
	lastVersionMu   sync.RWMutex      // Protects lastVersion map from concurrent access
}

// NewSecretIngester creates a new Secret ingester.
func NewSecretIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *SecretIngester {
	return &SecretIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("secret-ingester"),
		lastVersion:     make(map[string]string),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// Returns an error if handler registration fails.
func (i *SecretIngester) RegisterHandlers() error {
	informer := i.informerFactory.Core().V1().Secrets().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    i.onAdd,
		UpdateFunc: i.onUpdate,
		DeleteFunc: i.onDelete,
	})
	return err
}

// onAdd handles Secret add events.
// SECURITY: Only metadata and key names are processed, never data values.
func (i *SecretIngester) onAdd(obj interface{}) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		i.log.Error(nil, "received non-Secret object in add handler")
		return
	}

	// SECURITY: Never log secret data - only log metadata
	i.log.V(1).Info("Secret added", "name", secret.Name, "namespace", secret.Namespace, "type", secret.Type)

	// Track version for deduplication
	key := secret.Namespace + "/" + secret.Name
	i.lastVersionMu.Lock()
	i.lastVersion[key] = secret.ResourceVersion
	i.lastVersionMu.Unlock()

	i.sendEvent(transport.EventTypeAdded, secret)
}

// onUpdate handles Secret update events.
// SECURITY: Only metadata and key names are processed, never data values.
func (i *SecretIngester) onUpdate(oldObj, newObj interface{}) {
	secret, ok := newObj.(*corev1.Secret)
	if !ok {
		i.log.Error(nil, "received non-Secret object in update handler")
		return
	}

	// Skip if resourceVersion hasn't changed (duplicate event)
	key := secret.Namespace + "/" + secret.Name
	i.lastVersionMu.Lock()
	if i.lastVersion[key] == secret.ResourceVersion {
		i.lastVersionMu.Unlock()
		i.log.V(2).Info("skipping duplicate Secret update", "name", secret.Name, "namespace", secret.Namespace)
		return
	}
	i.lastVersion[key] = secret.ResourceVersion
	i.lastVersionMu.Unlock()

	// SECURITY: Never log secret data - only log metadata
	i.log.V(1).Info("Secret updated", "name", secret.Name, "namespace", secret.Namespace, "type", secret.Type)
	i.sendEvent(transport.EventTypeModified, secret)
}

// onDelete handles Secret delete events.
func (i *SecretIngester) onDelete(obj interface{}) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			i.log.Error(nil, "received unexpected object in delete handler")
			return
		}
		secret, ok = tombstone.Obj.(*corev1.Secret)
		if !ok {
			i.log.Error(nil, "tombstone contained non-Secret object")
			return
		}
	}

	i.log.V(1).Info("Secret deleted", "name", secret.Name, "namespace", secret.Namespace)

	// Clean up version tracking
	key := secret.Namespace + "/" + secret.Name
	i.lastVersionMu.Lock()
	delete(i.lastVersion, key)
	i.lastVersionMu.Unlock()

	i.sendDeleteEvent(secret)
}

// sendEvent sends a Secret event to the event channel.
// SECURITY: Only metadata and keys are sent via transport.NewSecretInfo().
// Data values are NEVER included - this is enforced by the SecretInfo struct.
func (i *SecretIngester) sendEvent(eventType transport.EventType, secret *corev1.Secret) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeSecret,
		UID:       string(secret.UID),
		Name:      secret.Name,
		Namespace: secret.Namespace,
		Object:    transport.NewSecretInfo(secret), // SECURITY: Only extracts metadata and keys
	}

	select {
	case i.config.EventChan <- event:
		// Event sent successfully
	default:
		i.log.Info("event channel full, dropping Secret event",
			"type", eventType,
			"name", secret.Name,
			"namespace", secret.Namespace)
	}
}

// sendDeleteEvent sends a Secret delete event.
func (i *SecretIngester) sendDeleteEvent(secret *corev1.Secret) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeSecret,
		UID:       string(secret.UID),
		Name:      secret.Name,
		Namespace: secret.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case i.config.EventChan <- event:
		// Event sent successfully
	default:
		i.log.Info("event channel full, dropping Secret delete event",
			"name", secret.Name,
			"namespace", secret.Namespace)
	}
}
