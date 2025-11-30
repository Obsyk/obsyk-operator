// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	obsykv1 "github.com/obsyk/obsyk-operator/api/v1"
	"github.com/obsyk/obsyk-operator/internal/auth"
	"github.com/obsyk/obsyk-operator/internal/transport"
)

const (
	// kubeSystemNamespace is used to get the cluster UID.
	kubeSystemNamespace = "kube-system"
)

// agentClient holds a transport client and its token manager
type agentClient struct {
	transport    *transport.Client
	tokenManager *auth.TokenManager
}

// ObsykAgentReconciler reconciles an ObsykAgent object.
type ObsykAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// agentClients holds a client per ObsykAgent (keyed by namespace/name).
	agentClients map[string]*agentClient

	// httpClient is shared across all token managers
	httpClient *http.Client
}

// NewObsykAgentReconciler creates a new reconciler.
func NewObsykAgentReconciler(client client.Client, scheme *runtime.Scheme) *ObsykAgentReconciler {
	return &ObsykAgentReconciler{
		Client:       client,
		Scheme:       scheme,
		agentClients: make(map[string]*agentClient),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles ObsykAgent reconciliation.
func (r *ObsykAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ObsykAgent instance
	agent := &obsykv1.ObsykAgent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, clean up client
			delete(r.agentClients, req.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling ObsykAgent", "name", agent.Name, "namespace", agent.Namespace)

	// Get or create transport client with OAuth2 authentication
	ac, err := r.getOrCreateAgentClient(ctx, agent)
	if err != nil {
		logger.Error(err, "failed to create agent client")
		r.setCondition(agent, obsykv1.ConditionTypeDegraded, metav1.ConditionTrue,
			"AuthenticationError", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, agent)
	}

	// Get cluster UID if not set
	if agent.Status.ClusterUID == "" {
		clusterUID, err := r.getClusterUID(ctx)
		if err != nil {
			logger.Error(err, "failed to get cluster UID")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
		agent.Status.ClusterUID = clusterUID
	}

	// Check if we need to send initial snapshot
	if agent.Status.LastSnapshotTime == nil {
		if err := r.sendSnapshot(ctx, agent, ac.transport); err != nil {
			logger.Error(err, "failed to send snapshot")
			r.setCondition(agent, obsykv1.ConditionTypeSyncing, metav1.ConditionFalse,
				"SnapshotFailed", err.Error())
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, agent)
		}
		now := metav1.Now()
		agent.Status.LastSnapshotTime = &now
		agent.Status.LastSyncTime = &now
	} else {
		// Send periodic heartbeat
		if err := r.sendHeartbeat(ctx, agent, ac.transport); err != nil {
			logger.Error(err, "failed to send heartbeat")
			r.setCondition(agent, obsykv1.ConditionTypeSyncing, metav1.ConditionFalse,
				"HeartbeatFailed", err.Error())
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, agent)
		}
		now := metav1.Now()
		agent.Status.LastSyncTime = &now
	}

	// Update resource counts
	counts, err := r.getResourceCounts(ctx)
	if err != nil {
		logger.Error(err, "failed to get resource counts")
	} else {
		agent.Status.ResourceCounts = counts
	}

	// Set healthy conditions
	r.setCondition(agent, obsykv1.ConditionTypeAvailable, metav1.ConditionTrue,
		"AgentRunning", "Agent is running and connected to platform")
	r.setCondition(agent, obsykv1.ConditionTypeSyncing, metav1.ConditionTrue,
		"SyncActive", "Agent is actively syncing data")

	// Update observed generation
	agent.Status.ObservedGeneration = agent.Generation

	// Update status
	if err := r.Status().Update(ctx, agent); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue for periodic sync based on syncInterval
	syncInterval := 5 * time.Minute
	if agent.Spec.SyncInterval.Duration > 0 {
		syncInterval = agent.Spec.SyncInterval.Duration
	}

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// getOrCreateAgentClient gets or creates an agent client with OAuth2 authentication.
func (r *ObsykAgentReconciler) getOrCreateAgentClient(ctx context.Context, agent *obsykv1.ObsykAgent) (*agentClient, error) {
	key := fmt.Sprintf("%s/%s", agent.Namespace, agent.Name)
	logger := log.FromContext(ctx)

	// Get credentials from secret
	creds, err := r.getCredentials(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("getting credentials: %w", err)
	}

	// Check if client exists
	if ac, ok := r.agentClients[key]; ok {
		// Update credentials in case they changed
		ac.tokenManager.UpdateCredentials(creds)
		return ac, nil
	}

	// Create new token manager
	tokenManager := auth.NewTokenManager(
		agent.Spec.PlatformURL,
		creds,
		r.httpClient,
		logger,
	)

	// Create new transport client
	transportClient := transport.NewClient(transport.ClientConfig{
		PlatformURL:   agent.Spec.PlatformURL,
		TokenProvider: tokenManager,
		Logger:        logger,
	})

	ac := &agentClient{
		transport:    transportClient,
		tokenManager: tokenManager,
	}
	r.agentClients[key] = ac

	return ac, nil
}

// getCredentials retrieves OAuth2 credentials from the referenced secret.
// Expected secret format:
//
//	data:
//	  client_id: <OAuth2 client ID from platform>
//	  private_key: <PEM-encoded ECDSA P-256 private key>
func (r *ObsykAgentReconciler) getCredentials(ctx context.Context, agent *obsykv1.ObsykAgent) (*auth.Credentials, error) {
	secret := &corev1.Secret{}
	secretRef := types.NamespacedName{
		Namespace: agent.Namespace,
		Name:      agent.Spec.CredentialsSecretRef.Name,
	}

	if err := r.Get(ctx, secretRef, secret); err != nil {
		return nil, fmt.Errorf("getting secret %s: %w", secretRef, err)
	}

	creds, err := auth.ParseCredentials(secret.Data)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials from secret %s: %w", secretRef, err)
	}

	return creds, nil
}

// getClusterUID retrieves the cluster UID from the kube-system namespace.
func (r *ObsykAgentReconciler) getClusterUID(ctx context.Context) (string, error) {
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: kubeSystemNamespace}, ns); err != nil {
		return "", fmt.Errorf("getting kube-system namespace: %w", err)
	}
	return string(ns.UID), nil
}

// sendSnapshot sends a full cluster state snapshot.
func (r *ObsykAgentReconciler) sendSnapshot(ctx context.Context, agent *obsykv1.ObsykAgent, client *transport.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("sending initial snapshot")

	// List all namespaces
	namespaces := &corev1.NamespaceList{}
	if err := r.List(ctx, namespaces); err != nil {
		return fmt.Errorf("listing namespaces: %w", err)
	}

	// List all pods
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods); err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}

	// List all services
	services := &corev1.ServiceList{}
	if err := r.List(ctx, services); err != nil {
		return fmt.Errorf("listing services: %w", err)
	}

	// Build snapshot payload
	payload := &transport.SnapshotPayload{
		ClusterUID:   agent.Status.ClusterUID,
		ClusterName:  agent.Spec.ClusterName,
		AgentVersion: "0.1.0", // TODO: get from build info
		Namespaces:   make([]transport.NamespaceInfo, 0, len(namespaces.Items)),
		Pods:         make([]transport.PodInfo, 0, len(pods.Items)),
		Services:     make([]transport.ServiceInfo, 0, len(services.Items)),
	}

	for i := range namespaces.Items {
		payload.Namespaces = append(payload.Namespaces, transport.NewNamespaceInfo(&namespaces.Items[i]))
	}
	for i := range pods.Items {
		payload.Pods = append(payload.Pods, transport.NewPodInfo(&pods.Items[i]))
	}
	for i := range services.Items {
		payload.Services = append(payload.Services, transport.NewServiceInfo(&services.Items[i]))
	}

	logger.Info("snapshot prepared",
		"namespaces", len(payload.Namespaces),
		"pods", len(payload.Pods),
		"services", len(payload.Services))

	return client.SendSnapshot(ctx, payload)
}

// sendHeartbeat sends a periodic health check to the platform.
func (r *ObsykAgentReconciler) sendHeartbeat(ctx context.Context, agent *obsykv1.ObsykAgent, client *transport.Client) error {
	logger := log.FromContext(ctx)

	payload := &transport.HeartbeatPayload{
		ClusterUID:   agent.Status.ClusterUID,
		AgentVersion: "0.1.0", // TODO: get from build info
	}

	logger.V(1).Info("sending heartbeat",
		"clusterUID", agent.Status.ClusterUID)

	return client.SendHeartbeat(ctx, payload)
}

// getResourceCounts returns counts of watched resources.
func (r *ObsykAgentReconciler) getResourceCounts(ctx context.Context) (*obsykv1.ResourceCounts, error) {
	namespaces := &corev1.NamespaceList{}
	if err := r.List(ctx, namespaces); err != nil {
		return nil, err
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods); err != nil {
		return nil, err
	}

	services := &corev1.ServiceList{}
	if err := r.List(ctx, services); err != nil {
		return nil, err
	}

	return &obsykv1.ResourceCounts{
		Namespaces: int32(len(namespaces.Items)),
		Pods:       int32(len(pods.Items)),
		Services:   int32(len(services.Items)),
	}, nil
}

// setCondition sets a condition on the agent status.
func (r *ObsykAgentReconciler) setCondition(agent *obsykv1.ObsykAgent, condType obsykv1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(condType),
		Status:             status,
		ObservedGeneration: agent.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	agent.Status.SetCondition(condition)
}

// findAgentsForResource returns reconcile requests for all ObsykAgents.
// This is used to trigger reconciliation when watched resources change.
func (r *ObsykAgentReconciler) findAgentsForResource(ctx context.Context, obj client.Object) []reconcile.Request {
	agents := &obsykv1.ObsykAgentList{}
	if err := r.List(ctx, agents); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(agents.Items))
	for _, agent := range agents.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      agent.Name,
				Namespace: agent.Namespace,
			},
		})
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ObsykAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&obsykv1.ObsykAgent{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForResource),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForResource),
		).
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForResource),
		).
		Complete(r)
}
