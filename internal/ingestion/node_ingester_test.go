// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/obsyk/obsyk-operator/internal/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestNodeIngester_OnAdd(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNodeIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.28.0",
				ContainerRuntimeVersion: "containerd://1.7.0",
				OSImage:                 "Ubuntu 22.04",
				Architecture:            "amd64",
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	_, err := clientset.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeAdded {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeAdded)
		}
		if event.Kind != transport.ResourceTypeNode {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNode)
		}
		if event.Name != "test-node" {
			t.Errorf("Name = %v, want %v", event.Name, "test-node")
		}
		if event.Namespace != "" {
			t.Errorf("Namespace = %v, want empty (nodes are cluster-scoped)", event.Namespace)
		}
		if event.UID != "node-uid-123" {
			t.Errorf("UID = %v, want %v", event.UID, "node-uid-123")
		}
		if event.Object == nil {
			t.Error("Object should not be nil for add event")
		}
		// Verify NodeInfo data
		nodeInfo, ok := event.Object.(transport.NodeInfo)
		if !ok {
			t.Errorf("Object is not NodeInfo: %T", event.Object)
		} else {
			if nodeInfo.Status != "Ready" {
				t.Errorf("Status = %v, want Ready", nodeInfo.Status)
			}
			if nodeInfo.KubeletVersion != "v1.28.0" {
				t.Errorf("KubeletVersion = %v, want v1.28.0", nodeInfo.KubeletVersion)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for add event")
	}
}

func TestNodeIngester_OnUpdate(t *testing.T) {
	// Create initial node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-node",
			UID:             "node-uid-123",
			ResourceVersion: "1",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(node)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNodeIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Update the node
	updatedNode := node.DeepCopy()
	updatedNode.ResourceVersion = "2"
	updatedNode.Labels = map[string]string{"updated": "true"}

	_, err := clientset.CoreV1().Nodes().Update(context.TODO(), updatedNode, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update node: %v", err)
	}

	// Wait for update event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeModified {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeModified)
		}
		if event.Kind != transport.ResourceTypeNode {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNode)
		}
		if event.Object == nil {
			t.Error("Object should not be nil for update event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}
}

func TestNodeIngester_OnDelete(t *testing.T) {
	// Create initial node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
		},
	}

	clientset := fake.NewSimpleClientset(node)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNodeIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Delete the node
	err := clientset.CoreV1().Nodes().Delete(context.TODO(), "test-node", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete node: %v", err)
	}

	// Wait for delete event
	select {
	case event := <-eventChan:
		if event.Type != transport.EventTypeDeleted {
			t.Errorf("Type = %v, want %v", event.Type, transport.EventTypeDeleted)
		}
		if event.Kind != transport.ResourceTypeNode {
			t.Errorf("Kind = %v, want %v", event.Kind, transport.ResourceTypeNode)
		}
		if event.Name != "test-node" {
			t.Errorf("Name = %v, want %v", event.Name, "test-node")
		}
		// Object should be nil for delete events
		if event.Object != nil {
			t.Error("Object should be nil for delete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for delete event")
	}
}

func TestNodeIngester_ChannelFull(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)
	// Create a channel with capacity 0 to simulate full channel
	eventChan := make(chan ResourceEvent)
	log := ctrl.Log.WithName("test")

	ingester := NewNodeIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Create a node - this should not block even though channel is full
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
		},
	}

	done := make(chan struct{})
	go func() {
		_, _ = clientset.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		close(done)
	}()

	// Should complete without blocking
	select {
	case <-done:
		// Success - event was dropped but didn't block
	case <-time.After(2 * time.Second):
		t.Error("create operation blocked when channel was full")
	}
}

func TestNodeIngester_SkipSameResourceVersion(t *testing.T) {
	// Create initial node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-node",
			UID:             "node-uid-123",
			ResourceVersion: "1",
		},
	}

	clientset := fake.NewSimpleClientset(node)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	eventChan := make(chan ResourceEvent, 10)
	log := ctrl.Log.WithName("test")

	ingester := NewNodeIngester(factory, IngesterConfig{EventChan: eventChan}, log)
	ingester.RegisterHandlers()

	// Start informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// Drain initial add event
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial add event")
	}

	// Manually call onUpdate with same resource version - should be skipped
	ingester.onUpdate(node, node)

	// Should not receive any event
	select {
	case event := <-eventChan:
		t.Errorf("should not receive event for same resource version, got: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Success - no event was sent
	}
}

func TestNodeIngester_RolesExtraction(t *testing.T) {
	testCases := []struct {
		name          string
		labels        map[string]string
		expectedRoles []string
	}{
		{
			name: "control-plane role",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			expectedRoles: []string{"control-plane"},
		},
		{
			name: "master role (legacy)",
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
			expectedRoles: []string{"control-plane"},
		},
		{
			name: "worker role",
			labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
			},
			expectedRoles: []string{"worker"},
		},
		{
			name: "custom role",
			labels: map[string]string{
				"node-role.kubernetes.io/gpu": "",
			},
			expectedRoles: []string{"gpu"},
		},
		{
			name:          "no roles",
			labels:        map[string]string{},
			expectedRoles: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: tc.labels,
				},
			}

			nodeInfo := transport.NewNodeInfo(node)

			if len(nodeInfo.Roles) != len(tc.expectedRoles) {
				t.Errorf("Roles count = %d, want %d", len(nodeInfo.Roles), len(tc.expectedRoles))
				return
			}

			for i, role := range nodeInfo.Roles {
				if role != tc.expectedRoles[i] {
					t.Errorf("Role[%d] = %s, want %s", i, role, tc.expectedRoles[i])
				}
			}
		})
	}
}

func TestNodeIngester_StatusDetection(t *testing.T) {
	testCases := []struct {
		name           string
		conditions     []corev1.NodeCondition
		expectedStatus string
	}{
		{
			name: "Ready node",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			expectedStatus: "Ready",
		},
		{
			name: "NotReady node",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
			expectedStatus: "NotReady",
		},
		{
			name: "Unknown status",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
			},
			expectedStatus: "NotReady",
		},
		{
			name:           "No conditions",
			conditions:     nil,
			expectedStatus: "NotReady",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Conditions: tc.conditions,
				},
			}

			nodeInfo := transport.NewNodeInfo(node)

			if nodeInfo.Status != tc.expectedStatus {
				t.Errorf("Status = %s, want %s", nodeInfo.Status, tc.expectedStatus)
			}
		})
	}
}
