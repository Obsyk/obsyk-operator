// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// Ingester is the interface implemented by all resource ingesters.
// This allows the Manager to work with ingesters polymorphically.
type Ingester interface {
	RegisterHandlers() error
}

// IngestersFromFactory creates all 20 resource ingesters from a shared informer factory.
// Returns a slice of Ingester interfaces that can be used by the Manager.
func IngestersFromFactory(factory informers.SharedInformerFactory, cfg IngesterConfig, log logr.Logger) []Ingester {
	return []Ingester{
		// Core v1 resources
		NewGenericIngester(PodConfig(factory), cfg, log),
		NewGenericIngester(ServiceConfig(factory), cfg, log),
		NewGenericIngester(NamespaceConfig(factory), cfg, log),
		NewGenericIngester(NodeConfig(factory), cfg, log),
		NewGenericIngester(ConfigMapConfig(factory), cfg, log),
		NewGenericIngester(SecretConfig(factory), cfg, log),
		NewGenericIngester(PVCConfig(factory), cfg, log),
		NewGenericIngester(ServiceAccountConfig(factory), cfg, log),
		NewGenericIngester(EventConfig(factory), cfg, log),

		// Apps v1 resources
		NewGenericIngester(DeploymentConfig(factory), cfg, log),
		NewGenericIngester(StatefulSetConfig(factory), cfg, log),
		NewGenericIngester(DaemonSetConfig(factory), cfg, log),

		// Batch v1 resources
		NewGenericIngester(JobConfig(factory), cfg, log),
		NewGenericIngester(CronJobConfig(factory), cfg, log),

		// Networking v1 resources
		NewGenericIngester(IngressConfig(factory), cfg, log),
		NewGenericIngester(NetworkPolicyConfig(factory), cfg, log),

		// RBAC v1 resources
		NewGenericIngester(RoleConfig(factory), cfg, log),
		NewGenericIngester(ClusterRoleConfig(factory), cfg, log),
		NewGenericIngester(RoleBindingConfig(factory), cfg, log),
		NewGenericIngester(ClusterRoleBindingConfig(factory), cfg, log),
	}
}

// =============================================================================
// Core v1 Resource Configurations
// =============================================================================

// PodConfig returns the configuration for Pod ingestion.
func PodConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Pod] {
	return ResourceConfig[*corev1.Pod]{
		Name:         "pod",
		ResourceType: transport.ResourceTypePod,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Pods().Informer() },
		ToInfo:       func(pod *corev1.Pod) interface{} { return transport.NewPodInfo(pod) },
		IsNamespaced: true,
	}
}

// ServiceConfig returns the configuration for Service ingestion.
func ServiceConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Service] {
	return ResourceConfig[*corev1.Service]{
		Name:         "service",
		ResourceType: transport.ResourceTypeService,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Services().Informer() },
		ToInfo:       func(svc *corev1.Service) interface{} { return transport.NewServiceInfo(svc) },
		IsNamespaced: true,
	}
}

// NamespaceConfig returns the configuration for Namespace ingestion.
func NamespaceConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Namespace] {
	return ResourceConfig[*corev1.Namespace]{
		Name:         "namespace",
		ResourceType: transport.ResourceTypeNamespace,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Namespaces().Informer() },
		ToInfo:       func(ns *corev1.Namespace) interface{} { return transport.NewNamespaceInfo(ns) },
		IsNamespaced: false,
	}
}

// NodeConfig returns the configuration for Node ingestion.
func NodeConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Node] {
	return ResourceConfig[*corev1.Node]{
		Name:         "node",
		ResourceType: transport.ResourceTypeNode,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Nodes().Informer() },
		ToInfo:       func(node *corev1.Node) interface{} { return transport.NewNodeInfo(node) },
		IsNamespaced: false,
	}
}

// ConfigMapConfig returns the configuration for ConfigMap ingestion.
// SECURITY: Only metadata and keys are sent, never data values.
func ConfigMapConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.ConfigMap] {
	return ResourceConfig[*corev1.ConfigMap]{
		Name:         "configmap",
		ResourceType: transport.ResourceTypeConfigMap,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().ConfigMaps().Informer() },
		ToInfo:       func(cm *corev1.ConfigMap) interface{} { return transport.NewConfigMapInfo(cm) },
		IsNamespaced: true,
	}
}

// SecretConfig returns the configuration for Secret ingestion.
// SECURITY: Only metadata is sent, NEVER secret data or values.
func SecretConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Secret] {
	return ResourceConfig[*corev1.Secret]{
		Name:         "secret",
		ResourceType: transport.ResourceTypeSecret,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Secrets().Informer() },
		ToInfo:       func(secret *corev1.Secret) interface{} { return transport.NewSecretInfo(secret) },
		IsNamespaced: true,
	}
}

// PVCConfig returns the configuration for PersistentVolumeClaim ingestion.
func PVCConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.PersistentVolumeClaim] {
	return ResourceConfig[*corev1.PersistentVolumeClaim]{
		Name:         "pvc",
		ResourceType: transport.ResourceTypePersistentVolumeClaim,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().PersistentVolumeClaims().Informer() },
		ToInfo:       func(pvc *corev1.PersistentVolumeClaim) interface{} { return transport.NewPVCInfo(pvc) },
		IsNamespaced: true,
	}
}

// ServiceAccountConfig returns the configuration for ServiceAccount ingestion.
func ServiceAccountConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.ServiceAccount] {
	return ResourceConfig[*corev1.ServiceAccount]{
		Name:         "serviceaccount",
		ResourceType: transport.ResourceTypeServiceAccount,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().ServiceAccounts().Informer() },
		ToInfo:       func(sa *corev1.ServiceAccount) interface{} { return transport.NewServiceAccountInfo(sa) },
		IsNamespaced: true,
	}
}

// EventConfig returns the configuration for Event ingestion.
func EventConfig(factory informers.SharedInformerFactory) ResourceConfig[*corev1.Event] {
	return ResourceConfig[*corev1.Event]{
		Name:         "event",
		ResourceType: transport.ResourceTypeEvent,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Core().V1().Events().Informer() },
		ToInfo:       func(event *corev1.Event) interface{} { return transport.NewEventInfo(event) },
		IsNamespaced: true,
	}
}

// =============================================================================
// Apps v1 Resource Configurations
// =============================================================================

// DeploymentConfig returns the configuration for Deployment ingestion.
func DeploymentConfig(factory informers.SharedInformerFactory) ResourceConfig[*appsv1.Deployment] {
	return ResourceConfig[*appsv1.Deployment]{
		Name:         "deployment",
		ResourceType: transport.ResourceTypeDeployment,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Apps().V1().Deployments().Informer() },
		ToInfo:       func(deploy *appsv1.Deployment) interface{} { return transport.NewDeploymentInfo(deploy) },
		IsNamespaced: true,
	}
}

// StatefulSetConfig returns the configuration for StatefulSet ingestion.
func StatefulSetConfig(factory informers.SharedInformerFactory) ResourceConfig[*appsv1.StatefulSet] {
	return ResourceConfig[*appsv1.StatefulSet]{
		Name:         "statefulset",
		ResourceType: transport.ResourceTypeStatefulSet,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Apps().V1().StatefulSets().Informer() },
		ToInfo:       func(sts *appsv1.StatefulSet) interface{} { return transport.NewStatefulSetInfo(sts) },
		IsNamespaced: true,
	}
}

// DaemonSetConfig returns the configuration for DaemonSet ingestion.
func DaemonSetConfig(factory informers.SharedInformerFactory) ResourceConfig[*appsv1.DaemonSet] {
	return ResourceConfig[*appsv1.DaemonSet]{
		Name:         "daemonset",
		ResourceType: transport.ResourceTypeDaemonSet,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Apps().V1().DaemonSets().Informer() },
		ToInfo:       func(ds *appsv1.DaemonSet) interface{} { return transport.NewDaemonSetInfo(ds) },
		IsNamespaced: true,
	}
}

// =============================================================================
// Batch v1 Resource Configurations
// =============================================================================

// JobConfig returns the configuration for Job ingestion.
func JobConfig(factory informers.SharedInformerFactory) ResourceConfig[*batchv1.Job] {
	return ResourceConfig[*batchv1.Job]{
		Name:         "job",
		ResourceType: transport.ResourceTypeJob,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Batch().V1().Jobs().Informer() },
		ToInfo:       func(job *batchv1.Job) interface{} { return transport.NewJobInfo(job) },
		IsNamespaced: true,
	}
}

// CronJobConfig returns the configuration for CronJob ingestion.
func CronJobConfig(factory informers.SharedInformerFactory) ResourceConfig[*batchv1.CronJob] {
	return ResourceConfig[*batchv1.CronJob]{
		Name:         "cronjob",
		ResourceType: transport.ResourceTypeCronJob,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Batch().V1().CronJobs().Informer() },
		ToInfo:       func(cj *batchv1.CronJob) interface{} { return transport.NewCronJobInfo(cj) },
		IsNamespaced: true,
	}
}

// =============================================================================
// Networking v1 Resource Configurations
// =============================================================================

// IngressConfig returns the configuration for Ingress ingestion.
func IngressConfig(factory informers.SharedInformerFactory) ResourceConfig[*networkingv1.Ingress] {
	return ResourceConfig[*networkingv1.Ingress]{
		Name:         "ingress",
		ResourceType: transport.ResourceTypeIngress,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Networking().V1().Ingresses().Informer() },
		ToInfo:       func(ing *networkingv1.Ingress) interface{} { return transport.NewIngressInfo(ing) },
		IsNamespaced: true,
	}
}

// NetworkPolicyConfig returns the configuration for NetworkPolicy ingestion.
func NetworkPolicyConfig(factory informers.SharedInformerFactory) ResourceConfig[*networkingv1.NetworkPolicy] {
	return ResourceConfig[*networkingv1.NetworkPolicy]{
		Name:         "networkpolicy",
		ResourceType: transport.ResourceTypeNetworkPolicy,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Networking().V1().NetworkPolicies().Informer() },
		ToInfo:       func(np *networkingv1.NetworkPolicy) interface{} { return transport.NewNetworkPolicyInfo(np) },
		IsNamespaced: true,
	}
}

// =============================================================================
// RBAC v1 Resource Configurations
// =============================================================================

// RoleConfig returns the configuration for Role ingestion.
func RoleConfig(factory informers.SharedInformerFactory) ResourceConfig[*rbacv1.Role] {
	return ResourceConfig[*rbacv1.Role]{
		Name:         "role",
		ResourceType: transport.ResourceTypeRole,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Rbac().V1().Roles().Informer() },
		ToInfo:       func(role *rbacv1.Role) interface{} { return transport.NewRoleInfo(role) },
		IsNamespaced: true,
	}
}

// ClusterRoleConfig returns the configuration for ClusterRole ingestion.
func ClusterRoleConfig(factory informers.SharedInformerFactory) ResourceConfig[*rbacv1.ClusterRole] {
	return ResourceConfig[*rbacv1.ClusterRole]{
		Name:         "clusterrole",
		ResourceType: transport.ResourceTypeClusterRole,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Rbac().V1().ClusterRoles().Informer() },
		ToInfo:       func(cr *rbacv1.ClusterRole) interface{} { return transport.NewClusterRoleInfo(cr) },
		IsNamespaced: false,
	}
}

// RoleBindingConfig returns the configuration for RoleBinding ingestion.
func RoleBindingConfig(factory informers.SharedInformerFactory) ResourceConfig[*rbacv1.RoleBinding] {
	return ResourceConfig[*rbacv1.RoleBinding]{
		Name:         "rolebinding",
		ResourceType: transport.ResourceTypeRoleBinding,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Rbac().V1().RoleBindings().Informer() },
		ToInfo:       func(rb *rbacv1.RoleBinding) interface{} { return transport.NewRoleBindingInfo(rb) },
		IsNamespaced: true,
	}
}

// ClusterRoleBindingConfig returns the configuration for ClusterRoleBinding ingestion.
func ClusterRoleBindingConfig(factory informers.SharedInformerFactory) ResourceConfig[*rbacv1.ClusterRoleBinding] {
	return ResourceConfig[*rbacv1.ClusterRoleBinding]{
		Name:         "clusterrolebinding",
		ResourceType: transport.ResourceTypeClusterRoleBinding,
		GetInformer:  func() cache.SharedIndexInformer { return factory.Rbac().V1().ClusterRoleBindings().Informer() },
		ToInfo:       func(crb *rbacv1.ClusterRoleBinding) interface{} { return transport.NewClusterRoleBindingInfo(crb) },
		IsNamespaced: false,
	}
}
