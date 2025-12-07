// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// DeploymentIngester watches Deployment resources and sends events to the event channel.
type DeploymentIngester struct {
	informerFactory informers.SharedInformerFactory
	config          IngesterConfig
	log             logr.Logger
}

// NewDeploymentIngester creates a new DeploymentIngester.
func NewDeploymentIngester(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) *DeploymentIngester {
	return &DeploymentIngester{
		informerFactory: factory,
		config:          cfg,
		log:             log.WithName("deployment-ingester"),
	}
}

// RegisterHandlers registers the event handlers with the informer.
// This must be called before starting the informer factory.
func (d *DeploymentIngester) RegisterHandlers() {
	informer := d.informerFactory.Apps().V1().Deployments().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    d.onAdd,
		UpdateFunc: d.onUpdate,
		DeleteFunc: d.onDelete,
	})
	if err != nil {
		d.log.Error(err, "failed to add event handler")
	}
}

// onAdd handles Deployment addition events.
func (d *DeploymentIngester) onAdd(obj interface{}) {
	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		d.log.Error(nil, "received non-Deployment object in add handler")
		return
	}

	d.log.V(2).Info("deployment added",
		"name", deploy.Name,
		"namespace", deploy.Namespace,
		"uid", deploy.UID)

	d.sendEvent(transport.EventTypeAdded, deploy)
}

// onUpdate handles Deployment update events.
func (d *DeploymentIngester) onUpdate(oldObj, newObj interface{}) {
	oldDeploy, ok := oldObj.(*appsv1.Deployment)
	if !ok {
		return
	}
	newDeploy, ok := newObj.(*appsv1.Deployment)
	if !ok {
		return
	}

	// Skip if resource version hasn't changed (no actual update)
	if oldDeploy.ResourceVersion == newDeploy.ResourceVersion {
		return
	}

	d.log.V(2).Info("deployment updated",
		"name", newDeploy.Name,
		"namespace", newDeploy.Namespace,
		"uid", newDeploy.UID)

	d.sendEvent(transport.EventTypeModified, newDeploy)
}

// onDelete handles Deployment deletion events.
func (d *DeploymentIngester) onDelete(obj interface{}) {
	// Handle DeletedFinalStateUnknown (object was deleted from cache before we saw the delete event)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		d.log.Error(nil, "received non-Deployment object in delete handler")
		return
	}

	d.log.V(2).Info("deployment deleted",
		"name", deploy.Name,
		"namespace", deploy.Namespace,
		"uid", deploy.UID)

	// For delete events, we only need identifying info, not full object
	d.sendDeleteEvent(deploy)
}

// sendEvent sends a Deployment event to the event channel.
func (d *DeploymentIngester) sendEvent(eventType transport.EventType, deploy *appsv1.Deployment) {
	event := ResourceEvent{
		Type:      eventType,
		Kind:      transport.ResourceTypeDeployment,
		UID:       string(deploy.UID),
		Name:      deploy.Name,
		Namespace: deploy.Namespace,
		Object:    transport.NewDeploymentInfo(deploy),
	}

	select {
	case d.config.EventChan <- event:
	default:
		d.log.Error(nil, "event channel full, dropping event",
			"type", eventType,
			"name", deploy.Name,
			"namespace", deploy.Namespace)
	}
}

// sendDeleteEvent sends a Deployment delete event (without full object data).
func (d *DeploymentIngester) sendDeleteEvent(deploy *appsv1.Deployment) {
	event := ResourceEvent{
		Type:      transport.EventTypeDeleted,
		Kind:      transport.ResourceTypeDeployment,
		UID:       string(deploy.UID),
		Name:      deploy.Name,
		Namespace: deploy.Namespace,
		Object:    nil, // No object data for deletes
	}

	select {
	case d.config.EventChan <- event:
	default:
		d.log.Error(nil, "event channel full, dropping delete event",
			"name", deploy.Name,
			"namespace", deploy.Namespace)
	}
}
