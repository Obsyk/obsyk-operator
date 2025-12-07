// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewNamespaceInfo(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-namespace",
			UID:    "ns-uid-123",
			Labels: map[string]string{"env": "test"},
			Annotations: map[string]string{
				"custom": "annotation",
			},
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}

	info := NewNamespaceInfo(ns)

	if info.UID != "ns-uid-123" {
		t.Errorf("UID = %s, want ns-uid-123", info.UID)
	}
	if info.Name != "test-namespace" {
		t.Errorf("Name = %s, want test-namespace", info.Name)
	}
	if info.Phase != "Active" {
		t.Errorf("Phase = %s, want Active", info.Phase)
	}
	if info.Labels["env"] != "test" {
		t.Errorf("Labels[env] = %s, want test", info.Labels["env"])
	}
}

func TestNewPodInfo(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			NodeName:           "node-1",
			ServiceAccountName: "default",
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx:latest"},
				{Name: "sidecar", Image: "envoy:v1"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	info := NewPodInfo(pod)

	if info.UID != "pod-uid-123" {
		t.Errorf("UID = %s, want pod-uid-123", info.UID)
	}
	if info.Name != "test-pod" {
		t.Errorf("Name = %s, want test-pod", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.NodeName != "node-1" {
		t.Errorf("NodeName = %s, want node-1", info.NodeName)
	}
	if info.ServiceAccount != "default" {
		t.Errorf("ServiceAccount = %s, want default", info.ServiceAccount)
	}
	if info.Phase != "Running" {
		t.Errorf("Phase = %s, want Running", info.Phase)
	}
	if len(info.Containers) != 2 {
		t.Errorf("Containers count = %d, want 2", len(info.Containers))
	}
	if info.Containers[0].Name != "main" {
		t.Errorf("Container[0].Name = %s, want main", info.Containers[0].Name)
	}
	if info.Containers[0].Image != "nginx:latest" {
		t.Errorf("Container[0].Image = %s, want nginx:latest", info.Containers[0].Image)
	}
}

func TestNewNodeInfo(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
				"kubernetes.io/os":                      "linux",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
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

	info := NewNodeInfo(node)

	if info.UID != "node-uid-123" {
		t.Errorf("UID = %s, want node-uid-123", info.UID)
	}
	if info.Name != "test-node" {
		t.Errorf("Name = %s, want test-node", info.Name)
	}
	if info.Status != "Ready" {
		t.Errorf("Status = %s, want Ready", info.Status)
	}
	if info.KubeletVersion != "v1.28.0" {
		t.Errorf("KubeletVersion = %s, want v1.28.0", info.KubeletVersion)
	}
	if info.ContainerRuntime != "containerd://1.7.0" {
		t.Errorf("ContainerRuntime = %s, want containerd://1.7.0", info.ContainerRuntime)
	}
	if info.OSImage != "Ubuntu 22.04" {
		t.Errorf("OSImage = %s, want Ubuntu 22.04", info.OSImage)
	}
	if info.Architecture != "amd64" {
		t.Errorf("Architecture = %s, want amd64", info.Architecture)
	}
	// Check roles extraction
	foundControlPlane := false
	for _, role := range info.Roles {
		if role == "control-plane" {
			foundControlPlane = true
			break
		}
	}
	if !foundControlPlane {
		t.Errorf("Expected control-plane role, got %v", info.Roles)
	}
}

func TestNewNodeInfo_NotReady(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	info := NewNodeInfo(node)

	if info.Status != "NotReady" {
		t.Errorf("Status = %s, want NotReady", info.Status)
	}
}

func TestNewNodeInfo_NoConditions(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-123",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{},
		},
	}

	info := NewNodeInfo(node)

	if info.Status != "NotReady" {
		t.Errorf("Status = %s, want NotReady (default)", info.Status)
	}
}

func TestNewNodeInfo_Roles(t *testing.T) {
	testCases := []struct {
		name          string
		labels        map[string]string
		expectedRoles []string
	}{
		{
			name: "control-plane",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			expectedRoles: []string{"control-plane"},
		},
		{
			name: "master (legacy)",
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
			expectedRoles: []string{"control-plane"},
		},
		{
			name: "worker",
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

			info := NewNodeInfo(node)

			if len(info.Roles) != len(tc.expectedRoles) {
				t.Errorf("Roles count = %d, want %d", len(info.Roles), len(tc.expectedRoles))
				return
			}

			for i, role := range info.Roles {
				if role != tc.expectedRoles[i] {
					t.Errorf("Role[%d] = %s, want %s", i, role, tc.expectedRoles[i])
				}
			}
		})
	}
}

func TestNewServiceInfo(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			UID:       "svc-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Selector:  map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80},
				{Name: "https", Protocol: corev1.ProtocolTCP, Port: 443},
			},
		},
	}

	info := NewServiceInfo(svc)

	if info.UID != "svc-uid-123" {
		t.Errorf("UID = %s, want svc-uid-123", info.UID)
	}
	if info.Name != "test-service" {
		t.Errorf("Name = %s, want test-service", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.ServiceType != "ClusterIP" {
		t.Errorf("ServiceType = %s, want ClusterIP", info.ServiceType)
	}
	if info.ClusterIP != "10.0.0.1" {
		t.Errorf("ClusterIP = %s, want 10.0.0.1", info.ClusterIP)
	}
	if len(info.Ports) != 2 {
		t.Errorf("Ports count = %d, want 2", len(info.Ports))
	}
	if info.Ports[0].Name != "http" {
		t.Errorf("Ports[0].Name = %s, want http", info.Ports[0].Name)
	}
	if info.Ports[0].Port != 80 {
		t.Errorf("Ports[0].Port = %d, want 80", info.Ports[0].Port)
	}
}

func TestNewDeploymentInfo(t *testing.T) {
	replicas := int32(5)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			UID:       "deploy-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     3,
			AvailableReplicas: 3,
			UpdatedReplicas:   5,
		},
	}

	info := NewDeploymentInfo(deploy)

	if info.UID != "deploy-uid-123" {
		t.Errorf("UID = %s, want deploy-uid-123", info.UID)
	}
	if info.Name != "test-deployment" {
		t.Errorf("Name = %s, want test-deployment", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.Replicas != 5 {
		t.Errorf("Replicas = %d, want 5", info.Replicas)
	}
	if info.ReadyReplicas != 3 {
		t.Errorf("ReadyReplicas = %d, want 3", info.ReadyReplicas)
	}
	if info.Strategy != "RollingUpdate" {
		t.Errorf("Strategy = %s, want RollingUpdate", info.Strategy)
	}
	if info.Image != "nginx:latest" {
		t.Errorf("Image = %s, want nginx:latest", info.Image)
	}
}

func TestNewDeploymentInfo_NilReplicas(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
			},
		},
	}

	info := NewDeploymentInfo(deploy)

	if info.Replicas != 1 {
		t.Errorf("Replicas = %d, want 1 (default)", info.Replicas)
	}
}

func TestNewStatefulSetInfo(t *testing.T) {
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
			UID:       "sts-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "test-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   2,
			CurrentReplicas: 3,
		},
	}

	info := NewStatefulSetInfo(sts)

	if info.UID != "sts-uid-123" {
		t.Errorf("UID = %s, want sts-uid-123", info.UID)
	}
	if info.Name != "test-statefulset" {
		t.Errorf("Name = %s, want test-statefulset", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.Replicas != 3 {
		t.Errorf("Replicas = %d, want 3", info.Replicas)
	}
	if info.ReadyReplicas != 2 {
		t.Errorf("ReadyReplicas = %d, want 2", info.ReadyReplicas)
	}
	if info.CurrentReplicas != 3 {
		t.Errorf("CurrentReplicas = %d, want 3", info.CurrentReplicas)
	}
	if info.ServiceName != "test-headless" {
		t.Errorf("ServiceName = %s, want test-headless", info.ServiceName)
	}
	if info.UpdateStrategy != "RollingUpdate" {
		t.Errorf("UpdateStrategy = %s, want RollingUpdate", info.UpdateStrategy)
	}
	if info.Image != "postgres:14" {
		t.Errorf("Image = %s, want postgres:14", info.Image)
	}
}

func TestNewStatefulSetInfo_NilReplicas(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "test-svc",
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "postgres:14"},
					},
				},
			},
		},
	}

	info := NewStatefulSetInfo(sts)

	if info.Replicas != 1 {
		t.Errorf("Replicas = %d, want 1 (default)", info.Replicas)
	}
}

func TestNewDaemonSetInfo(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
			UID:       "ds-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "fluentd:v1.14"},
					},
					NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            4,
			NumberAvailable:        4,
		},
	}

	info := NewDaemonSetInfo(ds)

	if info.UID != "ds-uid-123" {
		t.Errorf("UID = %s, want ds-uid-123", info.UID)
	}
	if info.Name != "test-daemonset" {
		t.Errorf("Name = %s, want test-daemonset", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.DesiredNumberScheduled != 5 {
		t.Errorf("DesiredNumberScheduled = %d, want 5", info.DesiredNumberScheduled)
	}
	if info.CurrentNumberScheduled != 5 {
		t.Errorf("CurrentNumberScheduled = %d, want 5", info.CurrentNumberScheduled)
	}
	if info.NumberReady != 4 {
		t.Errorf("NumberReady = %d, want 4", info.NumberReady)
	}
	if info.NumberAvailable != 4 {
		t.Errorf("NumberAvailable = %d, want 4", info.NumberAvailable)
	}
	if info.UpdateStrategy != "RollingUpdate" {
		t.Errorf("UpdateStrategy = %s, want RollingUpdate", info.UpdateStrategy)
	}
	if info.Image != "fluentd:v1.14" {
		t.Errorf("Image = %s, want fluentd:v1.14", info.Image)
	}
	if info.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Errorf("NodeSelector = %v, want kubernetes.io/os=linux", info.NodeSelector)
	}
}

func TestNewDaemonSetInfo_NoContainers(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	info := NewDaemonSetInfo(ds)

	if info.Image != "" {
		t.Errorf("Image = %s, want empty", info.Image)
	}
}

func TestFilterAnnotations(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		expected    map[string]string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    nil,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    nil,
		},
		{
			name: "filter kubectl annotations",
			annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"custom": "value",
			},
			expected: map[string]string{"custom": "value"},
		},
		{
			name: "filter kubernetes.io annotations",
			annotations: map[string]string{
				"kubernetes.io/change-cause": "deployment",
				"app.kubernetes.io/name":     "kept", // app.kubernetes.io is NOT filtered
				"custom":                     "value",
			},
			expected: map[string]string{
				"app.kubernetes.io/name": "kept",
				"custom":                 "value",
			},
		},
		{
			name: "all filtered returns nil",
			annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"kubernetes.io/description":                        "test",
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := filterAnnotations(tc.annotations)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Errorf("Result length = %d, want %d", len(result), len(tc.expected))
				return
			}

			for k, v := range tc.expected {
				if result[k] != v {
					t.Errorf("result[%s] = %s, want %s", k, result[k], v)
				}
			}
		})
	}
}
