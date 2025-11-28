// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventType represents the type of resource change.
type EventType string

const (
	EventTypeAdded   EventType = "ADDED"
	EventTypeUpdated EventType = "UPDATED"
	EventTypeDeleted EventType = "DELETED"
)

// ResourceType represents the type of Kubernetes resource.
type ResourceType string

const (
	ResourceTypePod       ResourceType = "Pod"
	ResourceTypeService   ResourceType = "Service"
	ResourceTypeNamespace ResourceType = "Namespace"
)

// SnapshotPayload represents a full cluster state snapshot.
type SnapshotPayload struct {
	// ClusterName is the human-friendly cluster identifier.
	ClusterName string `json:"clusterName"`

	// ClusterUID is the unique cluster identifier (kube-system namespace UID).
	ClusterUID string `json:"clusterUID"`

	// Timestamp when the snapshot was taken.
	Timestamp time.Time `json:"timestamp"`

	// Namespaces in the cluster.
	Namespaces []NamespaceInfo `json:"namespaces"`

	// Pods in the cluster.
	Pods []PodInfo `json:"pods"`

	// Services in the cluster.
	Services []ServiceInfo `json:"services"`
}

// EventPayload represents a single resource change event.
type EventPayload struct {
	// ClusterName is the human-friendly cluster identifier.
	ClusterName string `json:"clusterName"`

	// ClusterUID is the unique cluster identifier.
	ClusterUID string `json:"clusterUID"`

	// Timestamp when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// EventType indicates the type of change (ADDED, UPDATED, DELETED).
	EventType EventType `json:"eventType"`

	// ResourceType indicates what kind of resource changed.
	ResourceType ResourceType `json:"resourceType"`

	// Resource contains the resource data (one of Namespace, Pod, Service).
	Resource interface{} `json:"resource"`
}

// HeartbeatPayload represents a periodic health check.
type HeartbeatPayload struct {
	// ClusterName is the human-friendly cluster identifier.
	ClusterName string `json:"clusterName"`

	// ClusterUID is the unique cluster identifier.
	ClusterUID string `json:"clusterUID"`

	// Timestamp when the heartbeat was sent.
	Timestamp time.Time `json:"timestamp"`

	// ResourceCounts contains counts of watched resources.
	ResourceCounts ResourceCounts `json:"resourceCounts"`

	// Version of the operator.
	Version string `json:"version,omitempty"`
}

// ResourceCounts holds counts of watched resources.
type ResourceCounts struct {
	Namespaces int32 `json:"namespaces"`
	Pods       int32 `json:"pods"`
	Services   int32 `json:"services"`
}

// NamespaceInfo contains relevant namespace information.
type NamespaceInfo struct {
	Name              string            `json:"name"`
	UID               string            `json:"uid"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Phase             string            `json:"phase"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
}

// PodInfo contains relevant pod information.
type PodInfo struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Phase             string            `json:"phase"`
	NodeName          string            `json:"nodeName,omitempty"`
	ServiceAccount    string            `json:"serviceAccount,omitempty"`
	Containers        []ContainerInfo   `json:"containers,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
}

// ContainerInfo contains relevant container information.
type ContainerInfo struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// ServiceInfo contains relevant service information.
type ServiceInfo struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Type              string            `json:"type"`
	ClusterIP         string            `json:"clusterIP,omitempty"`
	Ports             []PortInfo        `json:"ports,omitempty"`
	Selector          map[string]string `json:"selector,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
}

// PortInfo contains service port information.
type PortInfo struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort,omitempty"`
}

// NewNamespaceInfo creates NamespaceInfo from a Kubernetes Namespace.
func NewNamespaceInfo(ns *corev1.Namespace) NamespaceInfo {
	return NamespaceInfo{
		Name:              ns.Name,
		UID:               string(ns.UID),
		Labels:            ns.Labels,
		Annotations:       filterAnnotations(ns.Annotations),
		Phase:             string(ns.Status.Phase),
		CreationTimestamp: ns.CreationTimestamp.Time,
	}
}

// NewPodInfo creates PodInfo from a Kubernetes Pod.
func NewPodInfo(pod *corev1.Pod) PodInfo {
	containers := make([]ContainerInfo, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		containers = append(containers, ContainerInfo{
			Name:  c.Name,
			Image: c.Image,
		})
	}

	return PodInfo{
		Name:              pod.Name,
		Namespace:         pod.Namespace,
		UID:               string(pod.UID),
		Labels:            pod.Labels,
		Annotations:       filterAnnotations(pod.Annotations),
		Phase:             string(pod.Status.Phase),
		NodeName:          pod.Spec.NodeName,
		ServiceAccount:    pod.Spec.ServiceAccountName,
		Containers:        containers,
		CreationTimestamp: pod.CreationTimestamp.Time,
	}
}

// NewServiceInfo creates ServiceInfo from a Kubernetes Service.
func NewServiceInfo(svc *corev1.Service) ServiceInfo {
	ports := make([]PortInfo, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, PortInfo{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
		})
	}

	return ServiceInfo{
		Name:              svc.Name,
		Namespace:         svc.Namespace,
		UID:               string(svc.UID),
		Labels:            svc.Labels,
		Annotations:       filterAnnotations(svc.Annotations),
		Type:              string(svc.Spec.Type),
		ClusterIP:         svc.Spec.ClusterIP,
		Ports:             ports,
		Selector:          svc.Spec.Selector,
		CreationTimestamp: svc.CreationTimestamp.Time,
	}
}

// filterAnnotations removes potentially sensitive or noisy annotations.
func filterAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}

	// List of annotation prefixes to exclude
	excludePrefixes := []string{
		"kubectl.kubernetes.io/",
		"kubernetes.io/",
	}

	filtered := make(map[string]string)
	for k, v := range annotations {
		exclude := false
		for _, prefix := range excludePrefixes {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				exclude = true
				break
			}
		}
		if !exclude {
			filtered[k] = v
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// Ensure metav1 is used for time handling compatibility.
var _ = metav1.Time{}
