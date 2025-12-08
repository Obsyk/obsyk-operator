// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/obsyk/obsyk-operator/internal/transport"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

const (
	// DefaultEventBufferSize is the default size of the event buffer.
	DefaultEventBufferSize = 1000

	// DefaultResyncPeriod is the default resync period for informers.
	// Set to 0 to disable periodic resync (rely on watch events only).
	DefaultResyncPeriod = 0

	// eventProcessTimeout is the timeout for processing a single event.
	eventProcessTimeout = 30 * time.Second

	// DefaultEventsPerSecond is the default rate limit for sending events.
	DefaultEventsPerSecond = 10.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 20
)

// Manager coordinates resource ingestion from Kubernetes and sends events to the platform.
// It manages SharedInformerFactory lifecycle and ensures proper startup/shutdown ordering.
type Manager struct {
	config ManagerConfig
	log    logr.Logger

	// Kubernetes client and informer factory
	clientset       kubernetes.Interface
	informerFactory informers.SharedInformerFactory

	// Event channel for aggregating events from all ingesters
	eventChan chan ResourceEvent

	// Rate limiter for sending events to the platform
	limiter *rate.Limiter

	// Individual ingesters
	podIngester                *PodIngester
	serviceIngester            *ServiceIngester
	namespaceIngester          *NamespaceIngester
	nodeIngester               *NodeIngester
	deploymentIngester         *DeploymentIngester
	statefulsetIngester        *StatefulSetIngester
	daemonsetIngester          *DaemonSetIngester
	jobIngester                *JobIngester
	cronjobIngester            *CronJobIngester
	ingressIngester            *IngressIngester
	networkpolicyIngester      *NetworkPolicyIngester
	configmapIngester          *ConfigMapIngester
	secretIngester             *SecretIngester
	pvcIngester                *PVCIngester
	serviceaccountIngester     *ServiceAccountIngester
	roleIngester               *RoleIngester
	clusterroleIngester        *ClusterRoleIngester
	rolebindingIngester        *RoleBindingIngester
	clusterrolebindingIngester *ClusterRoleBindingIngester
	eventIngester              *EventIngester

	// Lifecycle management
	mu       sync.RWMutex
	started  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	cancelFn context.CancelFunc
}

// NewManager creates a new ingestion manager.
func NewManager(clientset kubernetes.Interface, cfg ManagerConfig, log logr.Logger) *Manager {
	bufferSize := cfg.EventBufferSize
	if bufferSize <= 0 {
		bufferSize = DefaultEventBufferSize
	}

	// Configure rate limiter
	eventsPerSecond := DefaultEventsPerSecond
	burstSize := DefaultBurstSize
	if cfg.RateLimit != nil {
		if cfg.RateLimit.EventsPerSecond > 0 {
			eventsPerSecond = cfg.RateLimit.EventsPerSecond
		}
		if cfg.RateLimit.BurstSize > 0 {
			burstSize = cfg.RateLimit.BurstSize
		}
	}

	return &Manager{
		config:    cfg,
		log:       log.WithName("ingestion-manager"),
		clientset: clientset,
		eventChan: make(chan ResourceEvent, bufferSize),
		limiter:   rate.NewLimiter(rate.Limit(eventsPerSecond), burstSize),
	}
}

// Start begins watching Kubernetes resources and sending events to the platform.
// It blocks until the context is cancelled or Stop() is called.
// Returns an error if already started or if informer sync fails.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("manager already started")
	}
	m.started = true
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})

	// Create cancellable context for event processor
	procCtx, cancel := context.WithCancel(ctx)
	m.cancelFn = cancel

	m.log.Info("starting ingestion manager", "clusterUID", m.config.ClusterUID)

	// Create shared informer factory (resync disabled - we rely on watch events)
	// Must be set while holding the lock for thread-safety with GetCurrentState
	m.informerFactory = informers.NewSharedInformerFactory(m.clientset, DefaultResyncPeriod)

	// Create ingesters with event channel
	ingesterCfg := IngesterConfig{EventChan: m.eventChan}
	m.podIngester = NewPodIngester(m.informerFactory, ingesterCfg, m.log)
	m.serviceIngester = NewServiceIngester(m.informerFactory, ingesterCfg, m.log)
	m.namespaceIngester = NewNamespaceIngester(m.informerFactory, ingesterCfg, m.log)
	m.nodeIngester = NewNodeIngester(m.informerFactory, ingesterCfg, m.log)
	m.deploymentIngester = NewDeploymentIngester(m.informerFactory, ingesterCfg, m.log)
	m.statefulsetIngester = NewStatefulSetIngester(m.informerFactory, ingesterCfg, m.log)
	m.daemonsetIngester = NewDaemonSetIngester(m.informerFactory, ingesterCfg, m.log)
	m.jobIngester = NewJobIngester(m.informerFactory, ingesterCfg, m.log)
	m.cronjobIngester = NewCronJobIngester(m.informerFactory, ingesterCfg, m.log)
	m.ingressIngester = NewIngressIngester(m.informerFactory, ingesterCfg, m.log)
	m.networkpolicyIngester = NewNetworkPolicyIngester(m.informerFactory, ingesterCfg, m.log)
	m.configmapIngester = NewConfigMapIngester(m.informerFactory, ingesterCfg, m.log)
	m.secretIngester = NewSecretIngester(m.informerFactory, ingesterCfg, m.log)
	m.pvcIngester = NewPVCIngester(m.informerFactory, ingesterCfg, m.log)
	m.serviceaccountIngester = NewServiceAccountIngester(m.informerFactory, ingesterCfg, m.log)
	m.roleIngester = NewRoleIngester(m.informerFactory, ingesterCfg, m.log)
	m.clusterroleIngester = NewClusterRoleIngester(m.informerFactory, ingesterCfg, m.log)
	m.rolebindingIngester = NewRoleBindingIngester(m.informerFactory, ingesterCfg, m.log)
	m.clusterrolebindingIngester = NewClusterRoleBindingIngester(m.informerFactory, ingesterCfg, m.log)
	m.eventIngester = NewEventIngester(m.informerFactory, ingesterCfg, m.log)

	// Register event handlers before starting factory
	if err := m.podIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering pod handler: %w", err)
	}
	if err := m.serviceIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering service handler: %w", err)
	}
	if err := m.namespaceIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering namespace handler: %w", err)
	}
	if err := m.nodeIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering node handler: %w", err)
	}
	if err := m.deploymentIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering deployment handler: %w", err)
	}
	if err := m.statefulsetIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering statefulset handler: %w", err)
	}
	if err := m.daemonsetIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering daemonset handler: %w", err)
	}
	if err := m.jobIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering job handler: %w", err)
	}
	if err := m.cronjobIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering cronjob handler: %w", err)
	}
	if err := m.ingressIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering ingress handler: %w", err)
	}
	if err := m.networkpolicyIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering networkpolicy handler: %w", err)
	}
	if err := m.configmapIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering configmap handler: %w", err)
	}
	if err := m.secretIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering secret handler: %w", err)
	}
	if err := m.pvcIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering pvc handler: %w", err)
	}
	if err := m.serviceaccountIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering serviceaccount handler: %w", err)
	}
	if err := m.roleIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering role handler: %w", err)
	}
	if err := m.clusterroleIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering clusterrole handler: %w", err)
	}
	if err := m.rolebindingIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering rolebinding handler: %w", err)
	}
	if err := m.clusterrolebindingIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering clusterrolebinding handler: %w", err)
	}
	if err := m.eventIngester.RegisterHandlers(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("registering event handler: %w", err)
	}

	// Start event processor goroutine
	go m.processEvents(procCtx)

	// Start all informers (must be done before releasing lock to ensure factory is ready)
	m.informerFactory.Start(m.stopCh)
	m.mu.Unlock()

	// Wait for caches to sync before processing events
	m.log.Info("waiting for informer caches to sync")
	syncCtx, syncCancel := context.WithTimeout(ctx, 60*time.Second)
	defer syncCancel()

	if !m.waitForCacheSync(syncCtx) {
		m.Stop()
		return fmt.Errorf("timed out waiting for caches to sync")
	}
	m.log.Info("informer caches synced successfully")

	// Wait for stop signal
	select {
	case <-ctx.Done():
		m.log.Info("context cancelled, stopping manager")
	case <-m.stopCh:
		m.log.Info("stop signal received")
	}

	return nil
}

// Stop gracefully stops the manager and all ingesters.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return
	}

	m.log.Info("stopping ingestion manager")

	// Signal informers to stop
	close(m.stopCh)

	// Cancel event processor context
	if m.cancelFn != nil {
		m.cancelFn()
	}

	// Wait for event processor to finish (with timeout)
	select {
	case <-m.doneCh:
		m.log.V(1).Info("event processor stopped")
	case <-time.After(10 * time.Second):
		m.log.Info("timeout waiting for event processor to stop")
	}

	// Shutdown informer factory
	if m.informerFactory != nil {
		m.informerFactory.Shutdown()
	}

	m.started = false
	m.log.Info("ingestion manager stopped")
}

// IsStarted returns true if the manager is currently running.
func (m *Manager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// waitForCacheSync waits for all informer caches to sync.
func (m *Manager) waitForCacheSync(ctx context.Context) bool {
	syncs := m.informerFactory.WaitForCacheSync(ctx.Done())
	for informerType, synced := range syncs {
		if !synced {
			m.log.Error(nil, "informer cache sync failed", "type", informerType)
			return false
		}
	}
	return true
}

// processEvents reads events from the channel and sends them to the platform.
func (m *Manager) processEvents(ctx context.Context) {
	defer close(m.doneCh)

	m.log.V(1).Info("event processor started")

	for {
		select {
		case <-ctx.Done():
			// Drain remaining events with short timeout
			m.drainEvents()
			return

		case event, ok := <-m.eventChan:
			if !ok {
				return
			}
			m.sendEvent(ctx, event)
		}
	}
}

// drainEvents processes any remaining events in the buffer.
func (m *Manager) drainEvents() {
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	drained := 0
	for {
		select {
		case event, ok := <-m.eventChan:
			if !ok {
				return
			}
			m.sendEvent(drainCtx, event)
			drained++
		case <-drainCtx.Done():
			if drained > 0 {
				m.log.Info("drained remaining events", "count", drained)
			}
			return
		default:
			if drained > 0 {
				m.log.Info("drained remaining events", "count", drained)
			}
			return
		}
	}
}

// sendEvent sends a single event to the platform with rate limiting.
func (m *Manager) sendEvent(ctx context.Context, event ResourceEvent) {
	// Wait for rate limiter before sending
	if err := m.limiter.Wait(ctx); err != nil {
		// Context cancelled during rate limit wait
		m.log.V(1).Info("rate limit wait cancelled",
			"type", event.Type,
			"kind", event.Kind,
			"name", event.Name,
			"namespace", event.Namespace,
			"error", err.Error())
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, eventProcessTimeout)
	defer cancel()

	payload := &transport.EventPayload{
		ClusterUID: m.config.ClusterUID,
		Type:       string(event.Type),
		Kind:       string(event.Kind),
		UID:        event.UID,
		Name:       event.Name,
		Namespace:  event.Namespace,
		Object:     event.Object,
	}

	if err := m.config.EventSender.SendEvent(sendCtx, payload); err != nil {
		m.log.Error(err, "failed to send event",
			"type", event.Type,
			"kind", event.Kind,
			"name", event.Name,
			"namespace", event.Namespace)
		// Don't crash on individual event failures - log and continue
		return
	}

	m.log.V(1).Info("event sent successfully",
		"type", event.Type,
		"kind", event.Kind,
		"name", event.Name,
		"namespace", event.Namespace)
}

// GetCurrentState returns the current state from all informer caches.
// This is useful for sending an initial snapshot.
func (m *Manager) GetCurrentState() (*transport.SnapshotPayload, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started || m.informerFactory == nil {
		return nil, fmt.Errorf("manager not started")
	}

	// Get namespaces
	nsLister := m.informerFactory.Core().V1().Namespaces().Lister()
	namespaces, err := nsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	// Get pods
	podLister := m.informerFactory.Core().V1().Pods().Lister()
	pods, err := podLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	// Get services
	svcLister := m.informerFactory.Core().V1().Services().Lister()
	services, err := svcLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	// Get nodes
	nodeLister := m.informerFactory.Core().V1().Nodes().Lister()
	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	// Get deployments
	deployLister := m.informerFactory.Apps().V1().Deployments().Lister()
	deployments, err := deployLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	// Get statefulsets
	stsLister := m.informerFactory.Apps().V1().StatefulSets().Lister()
	statefulsets, err := stsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing statefulsets: %w", err)
	}

	// Get daemonsets
	dsLister := m.informerFactory.Apps().V1().DaemonSets().Lister()
	daemonsets, err := dsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing daemonsets: %w", err)
	}

	// Get jobs
	jobLister := m.informerFactory.Batch().V1().Jobs().Lister()
	jobs, err := jobLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}

	// Get cronjobs
	cronjobLister := m.informerFactory.Batch().V1().CronJobs().Lister()
	cronjobs, err := cronjobLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing cronjobs: %w", err)
	}

	// Get ingresses
	ingLister := m.informerFactory.Networking().V1().Ingresses().Lister()
	ingresses, err := ingLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing ingresses: %w", err)
	}

	// Get network policies
	npLister := m.informerFactory.Networking().V1().NetworkPolicies().Lister()
	networkPolicies, err := npLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing network policies: %w", err)
	}

	// Get configmaps (metadata only, no data values)
	cmLister := m.informerFactory.Core().V1().ConfigMaps().Lister()
	configmaps, err := cmLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing configmaps: %w", err)
	}

	// Get secrets (metadata only, NEVER data values - security critical)
	secretLister := m.informerFactory.Core().V1().Secrets().Lister()
	secrets, err := secretLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	// Get PVCs
	pvcLister := m.informerFactory.Core().V1().PersistentVolumeClaims().Lister()
	pvcs, err := pvcLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing pvcs: %w", err)
	}

	// Get ServiceAccounts
	saLister := m.informerFactory.Core().V1().ServiceAccounts().Lister()
	serviceaccounts, err := saLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing serviceaccounts: %w", err)
	}

	// Get Roles
	roleLister := m.informerFactory.Rbac().V1().Roles().Lister()
	roles, err := roleLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}

	// Get ClusterRoles
	clusterRoleLister := m.informerFactory.Rbac().V1().ClusterRoles().Lister()
	clusterRoles, err := clusterRoleLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing clusterroles: %w", err)
	}

	// Get RoleBindings
	rbLister := m.informerFactory.Rbac().V1().RoleBindings().Lister()
	roleBindings, err := rbLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing rolebindings: %w", err)
	}

	// Get ClusterRoleBindings
	crbLister := m.informerFactory.Rbac().V1().ClusterRoleBindings().Lister()
	clusterRoleBindings, err := crbLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing clusterrolebindings: %w", err)
	}

	// Get Events
	eventLister := m.informerFactory.Core().V1().Events().Lister()
	events, err := eventLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	// Convert to transport types
	payload := &transport.SnapshotPayload{
		ClusterUID:          m.config.ClusterUID,
		Namespaces:          make([]transport.NamespaceInfo, 0, len(namespaces)),
		Pods:                make([]transport.PodInfo, 0, len(pods)),
		Services:            make([]transport.ServiceInfo, 0, len(services)),
		Nodes:               make([]transport.NodeInfo, 0, len(nodes)),
		Deployments:         make([]transport.DeploymentInfo, 0, len(deployments)),
		StatefulSets:        make([]transport.StatefulSetInfo, 0, len(statefulsets)),
		DaemonSets:          make([]transport.DaemonSetInfo, 0, len(daemonsets)),
		Jobs:                make([]transport.JobInfo, 0, len(jobs)),
		CronJobs:            make([]transport.CronJobInfo, 0, len(cronjobs)),
		Ingresses:           make([]transport.IngressInfo, 0, len(ingresses)),
		NetworkPolicies:     make([]transport.NetworkPolicyInfo, 0, len(networkPolicies)),
		ConfigMaps:          make([]transport.ConfigMapInfo, 0, len(configmaps)),
		Secrets:             make([]transport.SecretInfo, 0, len(secrets)),
		PVCs:                make([]transport.PVCInfo, 0, len(pvcs)),
		ServiceAccounts:     make([]transport.ServiceAccountInfo, 0, len(serviceaccounts)),
		Roles:               make([]transport.RoleInfo, 0, len(roles)),
		ClusterRoles:        make([]transport.RoleInfo, 0, len(clusterRoles)),
		RoleBindings:        make([]transport.RoleBindingInfo, 0, len(roleBindings)),
		ClusterRoleBindings: make([]transport.RoleBindingInfo, 0, len(clusterRoleBindings)),
		Events:              make([]transport.EventInfo, 0, len(events)),
	}

	for _, ns := range namespaces {
		payload.Namespaces = append(payload.Namespaces, transport.NewNamespaceInfo(ns))
	}

	for _, pod := range pods {
		payload.Pods = append(payload.Pods, transport.NewPodInfo(pod))
	}

	for _, svc := range services {
		payload.Services = append(payload.Services, transport.NewServiceInfo(svc))
	}

	for _, node := range nodes {
		payload.Nodes = append(payload.Nodes, transport.NewNodeInfo(node))
	}

	for _, deploy := range deployments {
		payload.Deployments = append(payload.Deployments, transport.NewDeploymentInfo(deploy))
	}

	for _, sts := range statefulsets {
		payload.StatefulSets = append(payload.StatefulSets, transport.NewStatefulSetInfo(sts))
	}

	for _, ds := range daemonsets {
		payload.DaemonSets = append(payload.DaemonSets, transport.NewDaemonSetInfo(ds))
	}

	for _, job := range jobs {
		payload.Jobs = append(payload.Jobs, transport.NewJobInfo(job))
	}

	for _, cj := range cronjobs {
		payload.CronJobs = append(payload.CronJobs, transport.NewCronJobInfo(cj))
	}

	for _, ing := range ingresses {
		payload.Ingresses = append(payload.Ingresses, transport.NewIngressInfo(ing))
	}

	for _, np := range networkPolicies {
		payload.NetworkPolicies = append(payload.NetworkPolicies, transport.NewNetworkPolicyInfo(np))
	}

	// SECURITY: ConfigMaps and Secrets only get metadata and keys, never values
	for _, cm := range configmaps {
		payload.ConfigMaps = append(payload.ConfigMaps, transport.NewConfigMapInfo(cm))
	}

	for _, secret := range secrets {
		payload.Secrets = append(payload.Secrets, transport.NewSecretInfo(secret))
	}

	for _, pvc := range pvcs {
		payload.PVCs = append(payload.PVCs, transport.NewPVCInfo(pvc))
	}

	for _, sa := range serviceaccounts {
		payload.ServiceAccounts = append(payload.ServiceAccounts, transport.NewServiceAccountInfo(sa))
	}

	for _, role := range roles {
		payload.Roles = append(payload.Roles, transport.NewRoleInfo(role))
	}

	for _, cr := range clusterRoles {
		payload.ClusterRoles = append(payload.ClusterRoles, transport.NewClusterRoleInfo(cr))
	}

	for _, rb := range roleBindings {
		payload.RoleBindings = append(payload.RoleBindings, transport.NewRoleBindingInfo(rb))
	}

	for _, crb := range clusterRoleBindings {
		payload.ClusterRoleBindings = append(payload.ClusterRoleBindings, transport.NewClusterRoleBindingInfo(crb))
	}

	for _, event := range events {
		payload.Events = append(payload.Events, transport.NewEventInfo(event))
	}

	return payload, nil
}

// ResourceCounts holds counts of watched resources.
type ResourceCounts struct {
	Namespaces          int
	Pods                int
	Services            int
	Nodes               int
	Deployments         int
	StatefulSets        int
	DaemonSets          int
	Jobs                int
	CronJobs            int
	Ingresses           int
	NetworkPolicies     int
	ConfigMaps          int
	Secrets             int
	PVCs                int
	ServiceAccounts     int
	Roles               int
	ClusterRoles        int
	RoleBindings        int
	ClusterRoleBindings int
	Events              int
}

// GetResourceCounts returns counts of all watched resources from informer caches.
// This is more efficient than GetCurrentState when only counts are needed.
func (m *Manager) GetResourceCounts() (*ResourceCounts, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started || m.informerFactory == nil {
		return nil, fmt.Errorf("manager not started")
	}

	counts := &ResourceCounts{}

	// Core resources
	if namespaces, err := m.informerFactory.Core().V1().Namespaces().Lister().List(labels.Everything()); err == nil {
		counts.Namespaces = len(namespaces)
	}
	if pods, err := m.informerFactory.Core().V1().Pods().Lister().List(labels.Everything()); err == nil {
		counts.Pods = len(pods)
	}
	if services, err := m.informerFactory.Core().V1().Services().Lister().List(labels.Everything()); err == nil {
		counts.Services = len(services)
	}
	if nodes, err := m.informerFactory.Core().V1().Nodes().Lister().List(labels.Everything()); err == nil {
		counts.Nodes = len(nodes)
	}
	if configmaps, err := m.informerFactory.Core().V1().ConfigMaps().Lister().List(labels.Everything()); err == nil {
		counts.ConfigMaps = len(configmaps)
	}
	if secrets, err := m.informerFactory.Core().V1().Secrets().Lister().List(labels.Everything()); err == nil {
		counts.Secrets = len(secrets)
	}
	if pvcs, err := m.informerFactory.Core().V1().PersistentVolumeClaims().Lister().List(labels.Everything()); err == nil {
		counts.PVCs = len(pvcs)
	}
	if serviceaccounts, err := m.informerFactory.Core().V1().ServiceAccounts().Lister().List(labels.Everything()); err == nil {
		counts.ServiceAccounts = len(serviceaccounts)
	}
	if events, err := m.informerFactory.Core().V1().Events().Lister().List(labels.Everything()); err == nil {
		counts.Events = len(events)
	}

	// Apps resources
	if deployments, err := m.informerFactory.Apps().V1().Deployments().Lister().List(labels.Everything()); err == nil {
		counts.Deployments = len(deployments)
	}
	if statefulsets, err := m.informerFactory.Apps().V1().StatefulSets().Lister().List(labels.Everything()); err == nil {
		counts.StatefulSets = len(statefulsets)
	}
	if daemonsets, err := m.informerFactory.Apps().V1().DaemonSets().Lister().List(labels.Everything()); err == nil {
		counts.DaemonSets = len(daemonsets)
	}

	// Batch resources
	if jobs, err := m.informerFactory.Batch().V1().Jobs().Lister().List(labels.Everything()); err == nil {
		counts.Jobs = len(jobs)
	}
	if cronjobs, err := m.informerFactory.Batch().V1().CronJobs().Lister().List(labels.Everything()); err == nil {
		counts.CronJobs = len(cronjobs)
	}

	// Networking resources
	if ingresses, err := m.informerFactory.Networking().V1().Ingresses().Lister().List(labels.Everything()); err == nil {
		counts.Ingresses = len(ingresses)
	}
	if networkpolicies, err := m.informerFactory.Networking().V1().NetworkPolicies().Lister().List(labels.Everything()); err == nil {
		counts.NetworkPolicies = len(networkpolicies)
	}

	// RBAC resources
	if roles, err := m.informerFactory.Rbac().V1().Roles().Lister().List(labels.Everything()); err == nil {
		counts.Roles = len(roles)
	}
	if clusterroles, err := m.informerFactory.Rbac().V1().ClusterRoles().Lister().List(labels.Everything()); err == nil {
		counts.ClusterRoles = len(clusterroles)
	}
	if rolebindings, err := m.informerFactory.Rbac().V1().RoleBindings().Lister().List(labels.Everything()); err == nil {
		counts.RoleBindings = len(rolebindings)
	}
	if clusterrolebindings, err := m.informerFactory.Rbac().V1().ClusterRoleBindings().Lister().List(labels.Everything()); err == nil {
		counts.ClusterRoleBindings = len(clusterrolebindings)
	}

	return counts, nil
}
