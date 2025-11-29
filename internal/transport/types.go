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
	// ClusterUID is the unique cluster identifier (kube-system namespace UID).
	ClusterUID string `json:"cluster_uid"`

	// ClusterName is the human-friendly cluster identifier.
	ClusterName string `json:"cluster_name"`

	// KubernetesVersion is the cluster's K8s version.
	KubernetesVersion string `json:"kubernetes_version,omitempty"`

	// Platform is the cluster platform (e.g., eks, gke, kind).
	Platform string `json:"platform,omitempty"`

	// Region is the cluster region.
	Region string `json:"region,omitempty"`

	// AgentVersion is the operator version.
	AgentVersion string `json:"agent_version,omitempty"`

	// Namespaces in the cluster.
	Namespaces []NamespaceInfo `json:"namespaces"`

	// Pods in the cluster.
	Pods []PodInfo `json:"pods"`

	// Services in the cluster.
	Services []ServiceInfo `json:"services"`
}

// EventPayload represents a single resource change event.
type EventPayload struct {
	// ClusterUID is the unique cluster identifier.
	ClusterUID string `json:"cluster_uid"`

	// EventType indicates the type of change (added, modified, deleted).
	Type string `json:"type"`

	// Kind indicates what kind of resource changed (Namespace, Pod, Service).
	Kind string `json:"kind"`

	// UID is the resource's unique identifier.
	UID string `json:"uid"`

	// Name is the resource name.
	Name string `json:"name"`

	// Namespace is the resource namespace (empty for cluster-scoped resources).
	Namespace string `json:"namespace,omitempty"`

	// Object contains the full resource data for add/update, nil for delete.
	Object interface{} `json:"object,omitempty"`
}

// HeartbeatPayload represents a periodic health check.
type HeartbeatPayload struct {
	// ClusterUID is the unique cluster identifier.
	ClusterUID string `json:"cluster_uid"`

	// AgentVersion of the operator.
	AgentVersion string `json:"agent_version,omitempty"`
}

// ResourceCounts holds counts of watched resources.
type ResourceCounts struct {
	Namespaces int32 `json:"namespaces"`
	Pods       int32 `json:"pods"`
	Services   int32 `json:"services"`
}

// NamespaceInfo contains relevant namespace information.
type NamespaceInfo struct {
	UID          string            `json:"uid"`
	Name         string            `json:"name"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Phase        string            `json:"phase,omitempty"`
	K8sCreatedAt *time.Time        `json:"k8s_created_at,omitempty"`
}

// PodInfo contains relevant pod information.
type PodInfo struct {
	UID            string            `json:"uid"`
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	NodeName       string            `json:"node_name,omitempty"`
	ServiceAccount string            `json:"service_account,omitempty"`
	Containers     []ContainerInfo   `json:"containers,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	K8sCreatedAt   *time.Time        `json:"k8s_created_at,omitempty"`
}

// ContainerInfo contains relevant container information.
type ContainerInfo struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// ServiceInfo contains relevant service information.
type ServiceInfo struct {
	UID          string            `json:"uid"`
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	ServiceType  string            `json:"service_type,omitempty"`
	ClusterIP    string            `json:"cluster_ip,omitempty"`
	Ports        []PortInfo        `json:"ports,omitempty"`
	Selector     map[string]string `json:"selector,omitempty"`
	K8sCreatedAt *time.Time        `json:"k8s_created_at,omitempty"`
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
	createdAt := ns.CreationTimestamp.Time
	return NamespaceInfo{
		UID:          string(ns.UID),
		Name:         ns.Name,
		Labels:       ns.Labels,
		Annotations:  filterAnnotations(ns.Annotations),
		Phase:        string(ns.Status.Phase),
		K8sCreatedAt: &createdAt,
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

	createdAt := pod.CreationTimestamp.Time
	return PodInfo{
		UID:            string(pod.UID),
		Name:           pod.Name,
		Namespace:      pod.Namespace,
		Labels:         pod.Labels,
		Annotations:    filterAnnotations(pod.Annotations),
		NodeName:       pod.Spec.NodeName,
		ServiceAccount: pod.Spec.ServiceAccountName,
		Containers:     containers,
		Phase:          string(pod.Status.Phase),
		K8sCreatedAt:   &createdAt,
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

	createdAt := svc.CreationTimestamp.Time
	return ServiceInfo{
		UID:          string(svc.UID),
		Name:         svc.Name,
		Namespace:    svc.Namespace,
		Labels:       svc.Labels,
		Annotations:  filterAnnotations(svc.Annotations),
		ServiceType:  string(svc.Spec.Type),
		ClusterIP:    svc.Spec.ClusterIP,
		Ports:        ports,
		Selector:     svc.Spec.Selector,
		K8sCreatedAt: &createdAt,
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
