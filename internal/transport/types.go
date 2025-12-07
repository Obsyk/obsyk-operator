// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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
	ResourceTypePod         ResourceType = "Pod"
	ResourceTypeService     ResourceType = "Service"
	ResourceTypeNamespace   ResourceType = "Namespace"
	ResourceTypeNode        ResourceType = "Node"
	ResourceTypeDeployment  ResourceType = "Deployment"
	ResourceTypeStatefulSet ResourceType = "StatefulSet"
	ResourceTypeDaemonSet   ResourceType = "DaemonSet"
	ResourceTypeJob         ResourceType = "Job"
	ResourceTypeCronJob     ResourceType = "CronJob"
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

	// Nodes in the cluster.
	Nodes []NodeInfo `json:"nodes"`

	// Deployments in the cluster.
	Deployments []DeploymentInfo `json:"deployments"`

	// StatefulSets in the cluster.
	StatefulSets []StatefulSetInfo `json:"statefulsets"`

	// DaemonSets in the cluster.
	DaemonSets []DaemonSetInfo `json:"daemonsets"`

	// Jobs in the cluster.
	Jobs []JobInfo `json:"jobs"`

	// CronJobs in the cluster.
	CronJobs []CronJobInfo `json:"cronjobs"`
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
	Namespaces   int32 `json:"namespaces"`
	Pods         int32 `json:"pods"`
	Services     int32 `json:"services"`
	Nodes        int32 `json:"nodes"`
	Deployments  int32 `json:"deployments"`
	StatefulSets int32 `json:"statefulsets"`
	DaemonSets   int32 `json:"daemonsets"`
	Jobs         int32 `json:"jobs"`
	CronJobs     int32 `json:"cronjobs"`
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

// NodeInfo contains relevant node information.
type NodeInfo struct {
	UID              string            `json:"uid"`
	Name             string            `json:"name"`
	Labels           map[string]string `json:"labels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	Status           string            `json:"status"` // Ready, NotReady
	Roles            []string          `json:"roles,omitempty"`
	KubeletVersion   string            `json:"kubelet_version,omitempty"`
	ContainerRuntime string            `json:"container_runtime,omitempty"`
	OSImage          string            `json:"os_image,omitempty"`
	Architecture     string            `json:"architecture,omitempty"`
	CPUCapacity      string            `json:"cpu_capacity,omitempty"`
	MemoryCapacity   string            `json:"memory_capacity,omitempty"`
	PodCapacity      string            `json:"pod_capacity,omitempty"`
	K8sCreatedAt     *time.Time        `json:"k8s_created_at,omitempty"`
}

// DeploymentInfo contains relevant deployment information.
type DeploymentInfo struct {
	UID               string            `json:"uid"`
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	UpdatedReplicas   int32             `json:"updated_replicas"`
	Strategy          string            `json:"strategy,omitempty"`
	Selector          map[string]string `json:"selector,omitempty"`
	Image             string            `json:"image,omitempty"`
	K8sCreatedAt      *time.Time        `json:"k8s_created_at,omitempty"`
}

// StatefulSetInfo contains relevant statefulset information.
type StatefulSetInfo struct {
	UID             string            `json:"uid"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	Replicas        int32             `json:"replicas"`
	ReadyReplicas   int32             `json:"ready_replicas"`
	CurrentReplicas int32             `json:"current_replicas"`
	UpdateStrategy  string            `json:"update_strategy,omitempty"`
	ServiceName     string            `json:"service_name,omitempty"`
	Selector        map[string]string `json:"selector,omitempty"`
	Image           string            `json:"image,omitempty"`
	K8sCreatedAt    *time.Time        `json:"k8s_created_at,omitempty"`
}

// DaemonSetInfo contains relevant daemonset information.
type DaemonSetInfo struct {
	UID                    string            `json:"uid"`
	Name                   string            `json:"name"`
	Namespace              string            `json:"namespace"`
	Labels                 map[string]string `json:"labels,omitempty"`
	Annotations            map[string]string `json:"annotations,omitempty"`
	DesiredNumberScheduled int32             `json:"desired_number_scheduled"`
	CurrentNumberScheduled int32             `json:"current_number_scheduled"`
	NumberReady            int32             `json:"number_ready"`
	NumberAvailable        int32             `json:"number_available"`
	UpdateStrategy         string            `json:"update_strategy,omitempty"`
	Selector               map[string]string `json:"selector,omitempty"`
	NodeSelector           map[string]string `json:"node_selector,omitempty"`
	Image                  string            `json:"image,omitempty"`
	K8sCreatedAt           *time.Time        `json:"k8s_created_at,omitempty"`
}

// JobInfo contains relevant job information.
type JobInfo struct {
	UID            string            `json:"uid"`
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	Completions    int32             `json:"completions"`
	Parallelism    int32             `json:"parallelism"`
	Succeeded      int32             `json:"succeeded"`
	Failed         int32             `json:"failed"`
	Active         int32             `json:"active"`
	StartTime      *time.Time        `json:"start_time,omitempty"`
	CompletionTime *time.Time        `json:"completion_time,omitempty"`
	OwnerRef       string            `json:"owner_ref,omitempty"` // CronJob name if owned
	K8sCreatedAt   *time.Time        `json:"k8s_created_at,omitempty"`
}

// CronJobInfo contains relevant cronjob information.
type CronJobInfo struct {
	UID               string            `json:"uid"`
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Schedule          string            `json:"schedule"`
	Suspend           bool              `json:"suspend"`
	ConcurrencyPolicy string            `json:"concurrency_policy,omitempty"`
	LastScheduleTime  *time.Time        `json:"last_schedule_time,omitempty"`
	ActiveJobs        int32             `json:"active_jobs"`
	K8sCreatedAt      *time.Time        `json:"k8s_created_at,omitempty"`
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

// NewNodeInfo creates NodeInfo from a Kubernetes Node.
func NewNodeInfo(node *corev1.Node) NodeInfo {
	createdAt := node.CreationTimestamp.Time

	// Determine node status from conditions
	status := "NotReady"
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				status = "Ready"
			}
			break
		}
	}

	// Extract roles from labels
	var roles []string
	for label := range node.Labels {
		if label == "node-role.kubernetes.io/control-plane" || label == "node-role.kubernetes.io/master" {
			roles = append(roles, "control-plane")
		} else if label == "node-role.kubernetes.io/worker" {
			roles = append(roles, "worker")
		} else if len(label) > 24 && label[:24] == "node-role.kubernetes.io/" {
			roles = append(roles, label[24:])
		}
	}

	return NodeInfo{
		UID:              string(node.UID),
		Name:             node.Name,
		Labels:           node.Labels,
		Annotations:      filterAnnotations(node.Annotations),
		Status:           status,
		Roles:            roles,
		KubeletVersion:   node.Status.NodeInfo.KubeletVersion,
		ContainerRuntime: node.Status.NodeInfo.ContainerRuntimeVersion,
		OSImage:          node.Status.NodeInfo.OSImage,
		Architecture:     node.Status.NodeInfo.Architecture,
		CPUCapacity:      node.Status.Capacity.Cpu().String(),
		MemoryCapacity:   node.Status.Capacity.Memory().String(),
		PodCapacity:      node.Status.Capacity.Pods().String(),
		K8sCreatedAt:     &createdAt,
	}
}

// NewDeploymentInfo creates DeploymentInfo from a Kubernetes Deployment.
func NewDeploymentInfo(deploy *appsv1.Deployment) DeploymentInfo {
	createdAt := deploy.CreationTimestamp.Time

	// Extract primary container image
	var image string
	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		image = deploy.Spec.Template.Spec.Containers[0].Image
	}

	// Get replicas - defaults to 1 if not specified
	replicas := int32(1)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}

	// Get strategy type
	strategy := string(deploy.Spec.Strategy.Type)

	// Convert selector to map
	var selector map[string]string
	if deploy.Spec.Selector != nil {
		selector = deploy.Spec.Selector.MatchLabels
	}

	return DeploymentInfo{
		UID:               string(deploy.UID),
		Name:              deploy.Name,
		Namespace:         deploy.Namespace,
		Labels:            deploy.Labels,
		Annotations:       filterAnnotations(deploy.Annotations),
		Replicas:          replicas,
		ReadyReplicas:     deploy.Status.ReadyReplicas,
		AvailableReplicas: deploy.Status.AvailableReplicas,
		UpdatedReplicas:   deploy.Status.UpdatedReplicas,
		Strategy:          strategy,
		Selector:          selector,
		Image:             image,
		K8sCreatedAt:      &createdAt,
	}
}

// NewStatefulSetInfo creates StatefulSetInfo from a Kubernetes StatefulSet.
func NewStatefulSetInfo(sts *appsv1.StatefulSet) StatefulSetInfo {
	createdAt := sts.CreationTimestamp.Time

	// Extract primary container image
	var image string
	if len(sts.Spec.Template.Spec.Containers) > 0 {
		image = sts.Spec.Template.Spec.Containers[0].Image
	}

	// Get replicas - defaults to 1 if not specified
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	// Get update strategy type
	updateStrategy := string(sts.Spec.UpdateStrategy.Type)

	// Convert selector to map
	var selector map[string]string
	if sts.Spec.Selector != nil {
		selector = sts.Spec.Selector.MatchLabels
	}

	return StatefulSetInfo{
		UID:             string(sts.UID),
		Name:            sts.Name,
		Namespace:       sts.Namespace,
		Labels:          sts.Labels,
		Annotations:     filterAnnotations(sts.Annotations),
		Replicas:        replicas,
		ReadyReplicas:   sts.Status.ReadyReplicas,
		CurrentReplicas: sts.Status.CurrentReplicas,
		UpdateStrategy:  updateStrategy,
		ServiceName:     sts.Spec.ServiceName,
		Selector:        selector,
		Image:           image,
		K8sCreatedAt:    &createdAt,
	}
}

// NewDaemonSetInfo creates DaemonSetInfo from a Kubernetes DaemonSet.
func NewDaemonSetInfo(ds *appsv1.DaemonSet) DaemonSetInfo {
	createdAt := ds.CreationTimestamp.Time

	// Extract primary container image
	var image string
	if len(ds.Spec.Template.Spec.Containers) > 0 {
		image = ds.Spec.Template.Spec.Containers[0].Image
	}

	// Get update strategy type
	updateStrategy := string(ds.Spec.UpdateStrategy.Type)

	// Convert selector to map
	var selector map[string]string
	if ds.Spec.Selector != nil {
		selector = ds.Spec.Selector.MatchLabels
	}

	return DaemonSetInfo{
		UID:                    string(ds.UID),
		Name:                   ds.Name,
		Namespace:              ds.Namespace,
		Labels:                 ds.Labels,
		Annotations:            filterAnnotations(ds.Annotations),
		DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
		CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
		NumberReady:            ds.Status.NumberReady,
		NumberAvailable:        ds.Status.NumberAvailable,
		UpdateStrategy:         updateStrategy,
		Selector:               selector,
		NodeSelector:           ds.Spec.Template.Spec.NodeSelector,
		Image:                  image,
		K8sCreatedAt:           &createdAt,
	}
}

// NewJobInfo creates JobInfo from a Kubernetes Job.
func NewJobInfo(job *batchv1.Job) JobInfo {
	createdAt := job.CreationTimestamp.Time

	// Get completions - defaults to 1 if not specified
	completions := int32(1)
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}

	// Get parallelism - defaults to 1 if not specified
	parallelism := int32(1)
	if job.Spec.Parallelism != nil {
		parallelism = *job.Spec.Parallelism
	}

	// Get start time
	var startTime *time.Time
	if job.Status.StartTime != nil {
		t := job.Status.StartTime.Time
		startTime = &t
	}

	// Get completion time
	var completionTime *time.Time
	if job.Status.CompletionTime != nil {
		t := job.Status.CompletionTime.Time
		completionTime = &t
	}

	// Get owner reference (CronJob name if owned by one)
	var ownerRef string
	for _, ref := range job.OwnerReferences {
		if ref.Kind == "CronJob" {
			ownerRef = ref.Name
			break
		}
	}

	return JobInfo{
		UID:            string(job.UID),
		Name:           job.Name,
		Namespace:      job.Namespace,
		Labels:         job.Labels,
		Annotations:    filterAnnotations(job.Annotations),
		Completions:    completions,
		Parallelism:    parallelism,
		Succeeded:      job.Status.Succeeded,
		Failed:         job.Status.Failed,
		Active:         job.Status.Active,
		StartTime:      startTime,
		CompletionTime: completionTime,
		OwnerRef:       ownerRef,
		K8sCreatedAt:   &createdAt,
	}
}

// NewCronJobInfo creates CronJobInfo from a Kubernetes CronJob.
func NewCronJobInfo(cj *batchv1.CronJob) CronJobInfo {
	createdAt := cj.CreationTimestamp.Time

	// Get suspend status - defaults to false if not specified
	suspend := false
	if cj.Spec.Suspend != nil {
		suspend = *cj.Spec.Suspend
	}

	// Get last schedule time
	var lastScheduleTime *time.Time
	if cj.Status.LastScheduleTime != nil {
		t := cj.Status.LastScheduleTime.Time
		lastScheduleTime = &t
	}

	return CronJobInfo{
		UID:               string(cj.UID),
		Name:              cj.Name,
		Namespace:         cj.Namespace,
		Labels:            cj.Labels,
		Annotations:       filterAnnotations(cj.Annotations),
		Schedule:          cj.Spec.Schedule,
		Suspend:           suspend,
		ConcurrencyPolicy: string(cj.Spec.ConcurrencyPolicy),
		LastScheduleTime:  lastScheduleTime,
		ActiveJobs:        int32(len(cj.Status.Active)),
		K8sCreatedAt:      &createdAt,
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
