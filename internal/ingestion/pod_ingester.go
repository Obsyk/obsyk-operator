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

// PodIngester watches Pod resources and sends events to the event channel.
type PodIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewPodIngester creates a new PodIngester.
func NewPodIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *PodIngester {
	return &PodIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("pod-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
// Returns an error if handler registration fails.
func (p *PodIngester) RegisterHandlers() error {
	informer := p.informerFactory.Core().V1().Pods().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.onAdd,
		UpdateFunc: p.onUpdate,
		DeleteFunc: p.onDelete,
	})
	return err
}

// onAdd handles Pod addition events.
func (p *PodIngester) onAdd(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		p.log.Error(nil, "received non-Pod object in add handler")
		return
	}

	p.log.V(2).Info("pod added",
		"name", pod.Name,
		"namespace", pod.Namespace,
		"uid", pod.UID)

	p.sendEvent(transport.EventTypeAdded, pod)
}

// onUpdate handles Pod update events.
func (p *PodIngester) onUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}

	p.log.V(2).Info("pod updated",
		"name", newPod.Name,
		"namespace", newPod.Namespace,
		"uid", newPod.UID)

	p.sendEvent(transport.EventTypeModified, newPod)
}

// onDelete handles Pod deletion events.
func (p *PodIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		p.log.Error(nil, "received non-Pod object in delete handler")
		return
	}

	p.log.V(2).Info("pod deleted",
		"name", pod.Name,
		"namespace", pod.Namespace,
		"uid", pod.UID)

	// For delete events, we only need identifying info, not full object
	p.sendDeleteEvent(pod)
}

// sendEvent sends a Pod event to the event channel.
func (p *PodIngester) sendEvent(eventType transport.EventType, pod *corev1.Pod) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypePod,
		UID:       string(pod.UID),
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Object:    transport.NewPodInfo(pod),
	}

	select {
	case p.config.EventChan <- event:
	default:
		p.log.Info("event channel full, dropping event",
			"type", eventType,
			"name", pod.Name,
			"namespace", pod.Namespace)
	}
}

// sendDeleteEvent sends a Pod delete event (without full object data).
func (p *PodIngester) sendDeleteEvent(pod *corev1.Pod) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypePod,
		UID:       string(pod.UID),
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case p.config.EventChan <- event:
	default:
		p.log.Info("event channel full, dropping delete event",
			"name", pod.Name,
			"namespace", pod.Namespace)
	}
}
