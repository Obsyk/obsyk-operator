// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretReference references a Secret containing OAuth2 credentials.
// The Secret must contain the following keys:
//   - client_id: OAuth2 client ID from the platform
//   - private_key: PEM-encoded ECDSA P-256 private key for JWT signing
type SecretReference struct {
	// Name of the Secret containing OAuth2 credentials.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// ObsykAgentSpec defines the desired state of ObsykAgent.
type ObsykAgentSpec struct {
	// PlatformURL is the Obsyk SaaS platform endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	PlatformURL string `json:"platformURL"`

	// ClusterName is a human-friendly identifier for this cluster.
	// Used for display purposes in the Obsyk UI.
	// Does not need to be globally unique - uniqueness is enforced via ClusterUID.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ClusterName string `json:"clusterName"`

	// CredentialsSecretRef references a Secret containing OAuth2 credentials.
	// The Secret must contain 'client_id' (from platform) and 'private_key' (ECDSA P-256 PEM).
	// Generate keys locally, upload public key to platform, store private key in this Secret.
	// +kubebuilder:validation:Required
	CredentialsSecretRef SecretReference `json:"credentialsSecretRef"`

	// SyncInterval is how often to send heartbeat/full sync to the platform.
	// Defaults to 5m if not specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5m"
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
}

// ConditionType represents the type of condition.
type ConditionType string

const (
	// ConditionTypeAvailable indicates the agent is running and connected.
	ConditionTypeAvailable ConditionType = "Available"

	// ConditionTypeSyncing indicates the agent is actively syncing data.
	ConditionTypeSyncing ConditionType = "Syncing"

	// ConditionTypeDegraded indicates the agent is experiencing issues.
	ConditionTypeDegraded ConditionType = "Degraded"
)

// ObsykAgentStatus defines the observed state of ObsykAgent.
type ObsykAgentStatus struct {
	// ClusterUID is the unique identifier for this cluster.
	// Auto-detected from the kube-system namespace UID.
	// Used with organizationID (from API key) to ensure global uniqueness.
	// +kubebuilder:validation:Optional
	ClusterUID string `json:"clusterUID,omitempty"`

	// Conditions represent the latest available observations of the agent's state.
	// +kubebuilder:validation:Optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LastSyncTime is the timestamp of the last successful data sync to the platform.
	// +kubebuilder:validation:Optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastSnapshotTime is the timestamp of the last full snapshot sent to the platform.
	// +kubebuilder:validation:Optional
	LastSnapshotTime *metav1.Time `json:"lastSnapshotTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ResourceCounts tracks the number of watched resources.
	// +kubebuilder:validation:Optional
	ResourceCounts *ResourceCounts `json:"resourceCounts,omitempty"`
}

// ResourceCounts holds counts of watched resources.
type ResourceCounts struct {
	// Namespaces is the count of namespaces being watched.
	Namespaces int32 `json:"namespaces,omitempty"`

	// Pods is the count of pods being watched.
	Pods int32 `json:"pods,omitempty"`

	// Services is the count of services being watched.
	Services int32 `json:"services,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=oa
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster name"
// +kubebuilder:printcolumn:name="Platform",type="string",JSONPath=".spec.platformURL",description="Platform URL"
// +kubebuilder:printcolumn:name="Last Sync",type="date",JSONPath=".status.lastSyncTime",description="Last sync time"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ObsykAgent is the Schema for the obsykagents API.
// It configures the Obsyk agent to stream cluster metadata to the Obsyk platform.
type ObsykAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObsykAgentSpec   `json:"spec,omitempty"`
	Status ObsykAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObsykAgentList contains a list of ObsykAgent.
type ObsykAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObsykAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObsykAgent{}, &ObsykAgentList{})
}

// GetCondition returns the condition with the given type, or nil if not found.
func (s *ObsykAgentStatus) GetCondition(condType ConditionType) *metav1.Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == string(condType) {
			return &s.Conditions[i]
		}
	}
	return nil
}

// SetCondition sets the given condition, replacing any existing condition of the same type.
func (s *ObsykAgentStatus) SetCondition(condition metav1.Condition) {
	for i := range s.Conditions {
		if s.Conditions[i].Type == condition.Type {
			s.Conditions[i] = condition
			return
		}
	}
	s.Conditions = append(s.Conditions, condition)
}
