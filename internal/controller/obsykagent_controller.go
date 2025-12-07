// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	obsykv1 "github.com/obsyk/obsyk-operator/api/v1"
	"github.com/obsyk/obsyk-operator/internal/auth"
	"github.com/obsyk/obsyk-operator/internal/ingestion"
	"github.com/obsyk/obsyk-operator/internal/transport"
)

const (
	// kubeSystemNamespace is used to get the cluster UID.
	kubeSystemNamespace = "kube-system"
)

// agentClient holds a transport client, token manager, and ingestion manager
type agentClient struct {
	transport        *transport.Client
	tokenManager     *auth.TokenManager
	ingestionManager *ingestion.Manager
}

// ObsykAgentReconciler reconciles an ObsykAgent object.
type ObsykAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// APIReader is used for direct API calls without caching.
	// Used for secrets to avoid cluster-wide cache watches.
	APIReader client.Reader

	// Clientset is used for SharedInformerFactory in ingestion.
	Clientset kubernetes.Interface

	// agentClients holds a client per ObsykAgent (keyed by namespace/name).
	agentClients   map[string]*agentClient
	agentClientsMu sync.RWMutex // Protects agentClients map

	// httpClient is shared across all token managers
	httpClient *http.Client
}

// NewObsykAgentReconciler creates a new reconciler.
// apiReader should be mgr.GetAPIReader() for direct API calls without caching.
// clientset is the kubernetes.Interface used for SharedInformerFactory.
func NewObsykAgentReconciler(c client.Client, apiReader client.Reader, scheme *runtime.Scheme, clientset kubernetes.Interface) *ObsykAgentReconciler {
	return &ObsykAgentReconciler{
		Client:       c,
		Scheme:       scheme,
		APIReader:    apiReader,
		Clientset:    clientset,
		agentClients: make(map[string]*agentClient),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=obsyk.io,resources=obsykagents/finalizers,verbs=update
// Core resources
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch
// Apps resources
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=list;watch
// Batch resources
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=list;watch
// Networking resources
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=list;watch
// RBAC resources
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=list;watch
// Note: Operator-specific secrets are accessed via APIReader with namespace-scoped RBAC (Role, not ClusterRole)
// See charts/obsyk-operator/templates/role.yaml for the namespaced secret-reader Role

// Reconcile handles ObsykAgent reconciliation.
func (r *ObsykAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ObsykAgent instance
	agent := &obsykv1.ObsykAgent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, clean up client and stop ingestion
			r.agentClientsMu.Lock()
			if ac, ok := r.agentClients[req.String()]; ok {
				if ac.ingestionManager != nil {
					ac.ingestionManager.Stop()
				}
				delete(r.agentClients, req.String())
			}
			r.agentClientsMu.Unlock()
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

	// Check if we need to start ingestion manager and send initial snapshot
	if agent.Status.LastSnapshotTime == nil {
		// Start ingestion manager if not already running
		if ac.ingestionManager == nil {
			logger.Info("starting ingestion manager for agent")
			ac.ingestionManager = r.startIngestionManager(ctx, agent, ac.transport)
		}

		// Wait for ingestion manager to be ready before sending snapshot
		if ac.ingestionManager != nil && ac.ingestionManager.IsStarted() {
			// Use ingestion manager for complete snapshot with all 20 resource types
			if err := r.sendSnapshotFromManager(ctx, agent, ac); err != nil {
				logger.Error(err, "failed to send snapshot")
				r.setCondition(agent, obsykv1.ConditionTypeSyncing, metav1.ConditionFalse,
					"SnapshotFailed", err.Error())
				return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, agent)
			}
			now := metav1.Now()
			agent.Status.LastSnapshotTime = &now
			agent.Status.LastSyncTime = &now
		} else {
			// Ingestion manager not ready yet, requeue quickly
			logger.Info("waiting for ingestion manager to start")
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
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

	// Update resource counts from ingestion manager if available
	counts, err := r.getResourceCounts(ctx, ac)
	if err != nil {
		logger.Error(err, "failed to get resource counts")
	} else {
		agent.Status.ResourceCounts = counts
	}

	// Set healthy conditions and clear any degraded state
	r.setCondition(agent, obsykv1.ConditionTypeAvailable, metav1.ConditionTrue,
		"AgentRunning", "Agent is running and connected to platform")
	r.setCondition(agent, obsykv1.ConditionTypeSyncing, metav1.ConditionTrue,
		"SyncActive", "Agent is actively syncing data")
	r.setCondition(agent, obsykv1.ConditionTypeDegraded, metav1.ConditionFalse,
		"AgentHealthy", "Agent is operating normally")

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

	// Get credentials from secret (done outside lock to avoid holding lock during I/O)
	creds, err := r.getCredentials(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("getting credentials: %w", err)
	}

	// Check if client exists (fast path with read lock)
	r.agentClientsMu.RLock()
	if ac, ok := r.agentClients[key]; ok {
		r.agentClientsMu.RUnlock()
		// Update credentials in case they changed (TokenManager is thread-safe)
		ac.tokenManager.UpdateCredentials(creds)
		return ac, nil
	}
	r.agentClientsMu.RUnlock()

	// Slow path: need to create client (write lock)
	r.agentClientsMu.Lock()
	defer r.agentClientsMu.Unlock()

	// Double-check in case another goroutine created it while we waited for the lock
	if ac, ok := r.agentClients[key]; ok {
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

	// Use APIReader for direct API call without caching.
	// This avoids controller-runtime setting up a cluster-wide watch on secrets.
	if err := r.APIReader.Get(ctx, secretRef, secret); err != nil {
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

// startIngestionManager creates and starts an ingestion manager for the agent.
// It runs the manager in a background goroutine and returns immediately.
// Returns nil if Clientset is not configured (e.g., in tests).
func (r *ObsykAgentReconciler) startIngestionManager(ctx context.Context, agent *obsykv1.ObsykAgent, transportClient *transport.Client) *ingestion.Manager {
	logger := log.FromContext(ctx)

	// Clientset is required for SharedInformerFactory
	if r.Clientset == nil {
		logger.Info("clientset not configured, skipping ingestion manager")
		return nil
	}

	// Build rate limit config from agent spec
	var rateLimitCfg *ingestion.RateLimitConfig
	if agent.Spec.RateLimit != nil {
		rateLimitCfg = &ingestion.RateLimitConfig{
			EventsPerSecond: float64(agent.Spec.RateLimit.EventsPerSecond),
			BurstSize:       int(agent.Spec.RateLimit.BurstSize),
		}
	}

	cfg := ingestion.ManagerConfig{
		ClusterUID:  agent.Status.ClusterUID,
		EventSender: transportClient,
		RateLimit:   rateLimitCfg,
	}

	mgr := ingestion.NewManager(r.Clientset, cfg, logger)

	// Start manager in background goroutine
	go func() {
		if err := mgr.Start(ctx); err != nil {
			logger.Error(err, "ingestion manager stopped with error")
		}
	}()

	return mgr
}

// sendSnapshotFromManager sends a complete snapshot using the ingestion manager's cached state.
// This includes all 20 resource types instead of just 3.
func (r *ObsykAgentReconciler) sendSnapshotFromManager(ctx context.Context, agent *obsykv1.ObsykAgent, ac *agentClient) error {
	logger := log.FromContext(ctx)
	logger.Info("sending complete snapshot from ingestion manager")

	payload, err := ac.ingestionManager.GetCurrentState()
	if err != nil {
		return fmt.Errorf("getting current state from ingestion manager: %w", err)
	}

	// Set additional fields not available from manager
	payload.ClusterName = agent.Spec.ClusterName
	payload.AgentVersion = "0.1.0" // TODO: get from build info

	logger.Info("complete snapshot prepared",
		"namespaces", len(payload.Namespaces),
		"pods", len(payload.Pods),
		"services", len(payload.Services),
		"nodes", len(payload.Nodes),
		"deployments", len(payload.Deployments),
		"statefulsets", len(payload.StatefulSets),
		"daemonsets", len(payload.DaemonSets),
		"jobs", len(payload.Jobs),
		"cronjobs", len(payload.CronJobs),
		"ingresses", len(payload.Ingresses),
		"networkpolicies", len(payload.NetworkPolicies),
		"configmaps", len(payload.ConfigMaps),
		"secrets", len(payload.Secrets),
		"pvcs", len(payload.PVCs),
		"serviceaccounts", len(payload.ServiceAccounts),
		"roles", len(payload.Roles),
		"clusterroles", len(payload.ClusterRoles),
		"rolebindings", len(payload.RoleBindings),
		"clusterrolebindings", len(payload.ClusterRoleBindings),
		"events", len(payload.Events))

	return ac.transport.SendSnapshot(ctx, payload)
}

// getResourceCounts returns counts of watched resources.
// If ingestion manager is available, uses it for all 20 resource types.
// Otherwise falls back to basic counting of namespaces, pods, and services.
func (r *ObsykAgentReconciler) getResourceCounts(ctx context.Context, ac *agentClient) (*obsykv1.ResourceCounts, error) {
	// Try to get counts from ingestion manager if available
	if ac != nil && ac.ingestionManager != nil && ac.ingestionManager.IsStarted() {
		counts, err := ac.ingestionManager.GetResourceCounts()
		if err == nil {
			return &obsykv1.ResourceCounts{
				Namespaces:          int32(counts.Namespaces),
				Pods:                int32(counts.Pods),
				Services:            int32(counts.Services),
				Nodes:               int32(counts.Nodes),
				Deployments:         int32(counts.Deployments),
				StatefulSets:        int32(counts.StatefulSets),
				DaemonSets:          int32(counts.DaemonSets),
				Jobs:                int32(counts.Jobs),
				CronJobs:            int32(counts.CronJobs),
				Ingresses:           int32(counts.Ingresses),
				NetworkPolicies:     int32(counts.NetworkPolicies),
				ConfigMaps:          int32(counts.ConfigMaps),
				Secrets:             int32(counts.Secrets),
				PVCs:                int32(counts.PVCs),
				ServiceAccounts:     int32(counts.ServiceAccounts),
				Roles:               int32(counts.Roles),
				ClusterRoles:        int32(counts.ClusterRoles),
				RoleBindings:        int32(counts.RoleBindings),
				ClusterRoleBindings: int32(counts.ClusterRoleBindings),
				Events:              int32(counts.Events),
			}, nil
		}
		// Fall through to basic counting if manager fails
	}

	// Fallback: basic counting via API (only namespaces, pods, services)
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

// CheckPlatformHealth returns nil if all agent clients are healthy,
// or an error if any client is unhealthy.
// This is intended for use with Kubernetes health probes.
func (r *ObsykAgentReconciler) CheckPlatformHealth() error {
	r.agentClientsMu.RLock()
	defer r.agentClientsMu.RUnlock()

	// If no agents configured, consider healthy (operator is running)
	if len(r.agentClients) == 0 {
		return nil
	}

	// Check health of all agent clients
	for key, ac := range r.agentClients {
		if !ac.transport.IsHealthy() {
			status := ac.transport.GetHealthStatus()
			if status.LastError != nil {
				return fmt.Errorf("agent %s unhealthy: %v", key, status.LastError)
			}
			return fmt.Errorf("agent %s unhealthy: no successful connection yet", key)
		}
	}

	return nil
}

// CheckReady returns nil if the reconciler has successfully synced at least once,
// or an error if no initial sync has completed.
// This is intended for use with Kubernetes readiness probes.
func (r *ObsykAgentReconciler) CheckReady() error {
	r.agentClientsMu.RLock()
	defer r.agentClientsMu.RUnlock()

	// If no agents configured, consider ready (operator is running)
	if len(r.agentClients) == 0 {
		return nil
	}

	// Check that at least one agent has completed initial sync
	for _, ac := range r.agentClients {
		status := ac.transport.GetHealthStatus()
		if !status.LastHealthyTime.IsZero() {
			return nil // At least one agent has synced
		}
	}

	return fmt.Errorf("no agents have completed initial sync")
}
