// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"

	"github.com/obsyk/obsyk-operator/internal/transport"
)

// EventSender is the interface for sending events to the platform.
// This allows for easy mocking in tests.
type EventSender interface {
	SendEvent(ctx context.Context, payload *transport.EventPayload) error
}

// ResourceEvent represents a Kubernetes resource change event.
type ResourceEvent struct {
	// Type is the event type (ADDED, UPDATED, DELETED).
	Type transport.EventType

	// Kind is the resource kind (Pod, Service, Namespace).
	Kind transport.ResourceType

	// UID is the resource's unique identifier.
	UID string

	// Name is the resource name.
	Name string

	// Namespace is the resource namespace (empty for cluster-scoped).
	Namespace string

	// Object is the resource data (nil for delete events).
	Object interface{}
}

// ManagerConfig holds configuration for creating an IngestionManager.
type ManagerConfig struct {
	// ClusterUID is the unique cluster identifier.
	ClusterUID string

	// EventSender is used to send events to the platform.
	EventSender EventSender

	// EventBufferSize is the size of the event buffer channel.
	// Defaults to 1000 if not set.
	EventBufferSize int
}

// IngesterConfig holds common configuration for individual ingesters.
type IngesterConfig struct {
	// EventChan is the channel to send events to.
	EventChan chan<- ResourceEvent
}
