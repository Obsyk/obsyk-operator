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

// PVCIngester watches PersistentVolumeClaim resources and sends events to the event channel.
type PVCIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewPVCIngester creates a new PVCIngester.
func NewPVCIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *PVCIngester {
	return &PVCIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("pvc-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (i *PVCIngester) RegisterHandlers() {
	informer := i.informerFactory.Core().V1().PersistentVolumeClaims().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    i.onAdd,
		UpdateFunc: i.onUpdate,
		DeleteFunc: i.onDelete,
	})
	if err != nil {
		i.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles PVC addition events.
func (i *PVCIngester) onAdd(obj interface{}) {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		i.log.Error(nil, "received non-PVC object in add handler")
		return
	}

	i.log.V(2).Info("pvc added",
		"name", pvc.Name,
		"namespace", pvc.Namespace,
		"uid", pvc.UID)

	i.sendEvent(transport.EventTypeAdded, pvc)
}

// onUpdate handles PVC update events.
func (i *PVCIngester) onUpdate(oldObj, newObj interface{}) {
	oldPVC, ok := oldObj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return
	}
	newPVC, ok := newObj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldPVC.ResourceVersion == newPVC.ResourceVersion {
		return
	}

	i.log.V(2).Info("pvc updated",
		"name", newPVC.Name,
		"namespace", newPVC.Namespace,
		"uid", newPVC.UID,
		"phase", newPVC.Status.Phase)

	i.sendEvent(transport.EventTypeUpdated, newPVC)
}

// onDelete handles PVC deletion events.
func (i *PVCIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		i.log.Error(nil, "received non-PVC object in delete handler")
		return
	}

	i.log.V(2).Info("pvc deleted",
		"name", pvc.Name,
		"namespace", pvc.Namespace,
		"uid", pvc.UID)

	// For delete events, we only need identifying info, not full object
	i.sendDeleteEvent(pvc)
}

// sendEvent sends a PVC event to the event channel.
func (i *PVCIngester) sendEvent(eventType transport.EventType, pvc *corev1.PersistentVolumeClaim) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypePersistentVolumeClaim,
		UID:       string(pvc.UID),
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		Object:    transport.NewPVCInfo(pvc),
	}

	select {
	case i.config.EventChan <- event:
	default:
		i.log.Info("event channel full, dropping PVC event",
			"type", eventType,
			"name", pvc.Name,
			"namespace", pvc.Namespace)
	}
}

// sendDeleteEvent sends a PVC delete event (without full object data).
func (i *PVCIngester) sendDeleteEvent(pvc *corev1.PersistentVolumeClaim) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypePersistentVolumeClaim,
		UID:       string(pvc.UID),
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case i.config.EventChan <- event:
	default:
		i.log.Info("event channel full, dropping PVC delete event",
			"name", pvc.Name,
			"namespace", pvc.Namespace)
	}
}
