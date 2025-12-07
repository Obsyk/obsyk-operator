// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

func TestNewPodInfo_Enhanced(t *testing.T) {
	privileged := true
	runAsUser := int64(0)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "enhanced-pod",
			Namespace: "production",
			UID:       "pod-uid-enhanced",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			NodeName:           "node-1",
			ServiceAccountName: "app-sa",
			PriorityClassName:  "high-priority",
			InitContainers: []corev1.Container{
				{
					Name:  "init",
					Image: "busybox:latest",
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "nginx:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
					Ports: []corev1.ContainerPort{
						{ContainerPort: 80, Protocol: corev1.ProtocolTCP, Name: "http"},
						{ContainerPort: 443, Protocol: corev1.ProtocolTCP, Name: "https"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "config", MountPath: "/etc/config", ReadOnly: true},
						{Name: "data", MountPath: "/data", ReadOnly: false},
					},
					Env: []corev1.EnvVar{
						{Name: "DATABASE_URL", Value: "sensitive-value"},
						{Name: "API_KEY", Value: "secret-key"},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
						RunAsUser:  &runAsUser,
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"NET_ADMIN", "SYS_TIME"},
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}}},
				{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "app-pvc"}}},
				{Name: "secrets", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "app-secret"}}},
				{Name: "empty", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
		Status: corev1.PodStatus{
			Phase:    corev1.PodRunning,
			QOSClass: corev1.PodQOSBurstable,
			HostIP:   "192.168.1.10",
			PodIP:    "10.0.0.5",
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodInitialized, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 2,
					State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "init",
					Ready:        true,
					RestartCount: 0,
					State:        corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
				},
			},
		},
	}

	info := NewPodInfo(pod)

	// Test pod-level fields
	if info.QoSClass != "Burstable" {
		t.Errorf("QoSClass = %s, want Burstable", info.QoSClass)
	}
	if info.PriorityClassName != "high-priority" {
		t.Errorf("PriorityClassName = %s, want high-priority", info.PriorityClassName)
	}
	if info.HostIP != "192.168.1.10" {
		t.Errorf("HostIP = %s, want 192.168.1.10", info.HostIP)
	}
	if info.PodIP != "10.0.0.5" {
		t.Errorf("PodIP = %s, want 10.0.0.5", info.PodIP)
	}

	// Test conditions
	if len(info.Conditions) != 2 {
		t.Errorf("Conditions count = %d, want 2", len(info.Conditions))
	}

	// Test init containers
	if len(info.InitContainers) != 1 {
		t.Fatalf("InitContainers count = %d, want 1", len(info.InitContainers))
	}
	if info.InitContainers[0].Name != "init" {
		t.Errorf("InitContainer[0].Name = %s, want init", info.InitContainers[0].Name)
	}
	if info.InitContainers[0].State != "terminated" {
		t.Errorf("InitContainer[0].State = %s, want terminated", info.InitContainers[0].State)
	}

	// Test main container
	if len(info.Containers) != 1 {
		t.Fatalf("Containers count = %d, want 1", len(info.Containers))
	}
	c := info.Containers[0]
	if c.State != "running" {
		t.Errorf("Container.State = %s, want running", c.State)
	}
	if !c.Ready {
		t.Error("Container.Ready should be true")
	}
	if c.RestartCount != 2 {
		t.Errorf("Container.RestartCount = %d, want 2", c.RestartCount)
	}

	// Test resources
	if c.Resources.CPURequest != "100m" {
		t.Errorf("Resources.CPURequest = %s, want 100m", c.Resources.CPURequest)
	}
	if c.Resources.MemoryRequest != "256Mi" {
		t.Errorf("Resources.MemoryRequest = %s, want 256Mi", c.Resources.MemoryRequest)
	}
	if c.Resources.CPULimit != "500m" {
		t.Errorf("Resources.CPULimit = %s, want 500m", c.Resources.CPULimit)
	}
	if c.Resources.MemoryLimit != "512Mi" {
		t.Errorf("Resources.MemoryLimit = %s, want 512Mi", c.Resources.MemoryLimit)
	}

	// Test ports
	if len(c.Ports) != 2 {
		t.Fatalf("Ports count = %d, want 2", len(c.Ports))
	}
	if c.Ports[0].ContainerPort != 80 {
		t.Errorf("Ports[0].ContainerPort = %d, want 80", c.Ports[0].ContainerPort)
	}
	if c.Ports[0].Name != "http" {
		t.Errorf("Ports[0].Name = %s, want http", c.Ports[0].Name)
	}

	// Test volume mounts
	if len(c.VolumeMounts) != 2 {
		t.Fatalf("VolumeMounts count = %d, want 2", len(c.VolumeMounts))
	}
	if c.VolumeMounts[0].Name != "config" {
		t.Errorf("VolumeMounts[0].Name = %s, want config", c.VolumeMounts[0].Name)
	}
	if !c.VolumeMounts[0].ReadOnly {
		t.Error("VolumeMounts[0].ReadOnly should be true")
	}

	// Test env var names (security: verify NO values)
	if len(c.EnvVarNames) != 2 {
		t.Fatalf("EnvVarNames count = %d, want 2", len(c.EnvVarNames))
	}
	if c.EnvVarNames[0] != "DATABASE_URL" {
		t.Errorf("EnvVarNames[0] = %s, want DATABASE_URL", c.EnvVarNames[0])
	}
	// SECURITY: Verify values are NOT present
	for _, name := range c.EnvVarNames {
		if name == "sensitive-value" || name == "secret-key" {
			t.Fatalf("SECURITY VIOLATION: Env var value found in EnvVarNames")
		}
	}

	// Test security context
	if c.SecurityContext == nil {
		t.Fatal("SecurityContext should not be nil")
	}
	if c.SecurityContext.Privileged == nil || !*c.SecurityContext.Privileged {
		t.Error("SecurityContext.Privileged should be true")
	}
	if c.SecurityContext.RunAsRoot == nil || !*c.SecurityContext.RunAsRoot {
		t.Error("SecurityContext.RunAsRoot should be true (runAsUser=0)")
	}
	if len(c.SecurityContext.Capabilities) != 2 {
		t.Errorf("SecurityContext.Capabilities count = %d, want 2", len(c.SecurityContext.Capabilities))
	}

	// Test volumes
	if len(info.Volumes) != 4 {
		t.Fatalf("Volumes count = %d, want 4", len(info.Volumes))
	}
	volumeTypes := map[string]string{}
	volumeSources := map[string]string{}
	for _, v := range info.Volumes {
		volumeTypes[v.Name] = v.Type
		volumeSources[v.Name] = v.Source
	}
	if volumeTypes["config"] != "configMap" {
		t.Errorf("Volume config type = %s, want configMap", volumeTypes["config"])
	}
	if volumeSources["config"] != "app-config" {
		t.Errorf("Volume config source = %s, want app-config", volumeSources["config"])
	}
	if volumeTypes["data"] != "pvc" {
		t.Errorf("Volume data type = %s, want pvc", volumeTypes["data"])
	}
	if volumeSources["data"] != "app-pvc" {
		t.Errorf("Volume data source = %s, want app-pvc", volumeSources["data"])
	}
	if volumeTypes["secrets"] != "secret" {
		t.Errorf("Volume secrets type = %s, want secret", volumeTypes["secrets"])
	}
	if volumeTypes["empty"] != "emptyDir" {
		t.Errorf("Volume empty type = %s, want emptyDir", volumeTypes["empty"])
	}
}

func TestNewPodInfo_ContainerStates(t *testing.T) {
	tests := []struct {
		name          string
		state         corev1.ContainerState
		expectedState string
	}{
		{
			name:          "running",
			state:         corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			expectedState: "running",
		},
		{
			name:          "waiting",
			state:         corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}},
			expectedState: "waiting",
		},
		{
			name:          "terminated",
			state:         corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
			expectedState: "terminated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "main", State: tt.state},
					},
				},
			}

			info := NewPodInfo(pod)

			if len(info.Containers) == 0 {
				t.Fatal("Expected at least one container")
			}
			if info.Containers[0].State != tt.expectedState {
				t.Errorf("State = %s, want %s", info.Containers[0].State, tt.expectedState)
			}
		})
	}
}

func TestNewPodInfo_VolumeTypes(t *testing.T) {
	tests := []struct {
		name           string
		volume         corev1.Volume
		expectedType   string
		expectedSource string
	}{
		{
			name:           "emptyDir",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			expectedType:   "emptyDir",
			expectedSource: "",
		},
		{
			name:           "configMap",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"}}}},
			expectedType:   "configMap",
			expectedSource: "my-config",
		},
		{
			name:           "secret",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"}}},
			expectedType:   "secret",
			expectedSource: "my-secret",
		},
		{
			name:           "pvc",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "my-pvc"}}},
			expectedType:   "pvc",
			expectedSource: "my-pvc",
		},
		{
			name:           "hostPath",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log"}}},
			expectedType:   "hostPath",
			expectedSource: "/var/log",
		},
		{
			name:           "projected",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{}}},
			expectedType:   "projected",
			expectedSource: "",
		},
		{
			name:           "downwardAPI",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{DownwardAPI: &corev1.DownwardAPIVolumeSource{}}},
			expectedType:   "downwardAPI",
			expectedSource: "",
		},
		{
			name:           "csi",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "csi.storage.k8s.io"}}},
			expectedType:   "csi",
			expectedSource: "csi.storage.k8s.io",
		},
		{
			name:           "nfs",
			volume:         corev1.Volume{Name: "test", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "nfs.example.com", Path: "/exports"}}},
			expectedType:   "nfs",
			expectedSource: "nfs.example.com:/exports",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
					Volumes:    []corev1.Volume{tt.volume},
				},
			}

			info := NewPodInfo(pod)

			if len(info.Volumes) != 1 {
				t.Fatalf("Expected 1 volume, got %d", len(info.Volumes))
			}
			if info.Volumes[0].Type != tt.expectedType {
				t.Errorf("Type = %s, want %s", info.Volumes[0].Type, tt.expectedType)
			}
			if info.Volumes[0].Source != tt.expectedSource {
				t.Errorf("Source = %s, want %s", info.Volumes[0].Source, tt.expectedSource)
			}
		})
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

func TestNewJobInfo(t *testing.T) {
	completions := int32(3)
	parallelism := int32(2)
	startTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid-123",
			Labels:    map[string]string{"app": "test"},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "CronJob",
					Name: "parent-cronjob",
				},
			},
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
			Parallelism: &parallelism,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
			Failed:    0,
			Active:    2,
			StartTime: &startTime,
		},
	}

	info := NewJobInfo(job)

	if info.UID != "job-uid-123" {
		t.Errorf("UID = %s, want job-uid-123", info.UID)
	}
	if info.Name != "test-job" {
		t.Errorf("Name = %s, want test-job", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.Completions != 3 {
		t.Errorf("Completions = %d, want 3", info.Completions)
	}
	if info.Parallelism != 2 {
		t.Errorf("Parallelism = %d, want 2", info.Parallelism)
	}
	if info.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", info.Succeeded)
	}
	if info.Active != 2 {
		t.Errorf("Active = %d, want 2", info.Active)
	}
	if info.StartTime == nil {
		t.Error("StartTime should not be nil")
	}
	if info.OwnerRef != "parent-cronjob" {
		t.Errorf("OwnerRef = %s, want parent-cronjob", info.OwnerRef)
	}
}

func TestNewJobInfo_Defaults(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{},
	}

	info := NewJobInfo(job)

	if info.Completions != 1 {
		t.Errorf("Completions = %d, want 1 (default)", info.Completions)
	}
	if info.Parallelism != 1 {
		t.Errorf("Parallelism = %d, want 1 (default)", info.Parallelism)
	}
	if info.OwnerRef != "" {
		t.Errorf("OwnerRef = %s, want empty", info.OwnerRef)
	}
}

func TestNewCronJobInfo(t *testing.T) {
	suspend := false
	lastSchedule := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "default",
			UID:       "cj-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          "*/5 * * * *",
			Suspend:           &suspend,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
		},
		Status: batchv1.CronJobStatus{
			LastScheduleTime: &lastSchedule,
			Active: []corev1.ObjectReference{
				{Name: "job-1"},
				{Name: "job-2"},
			},
		},
	}

	info := NewCronJobInfo(cj)

	if info.UID != "cj-uid-123" {
		t.Errorf("UID = %s, want cj-uid-123", info.UID)
	}
	if info.Name != "test-cronjob" {
		t.Errorf("Name = %s, want test-cronjob", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.Schedule != "*/5 * * * *" {
		t.Errorf("Schedule = %s, want */5 * * * *", info.Schedule)
	}
	if info.Suspend != false {
		t.Errorf("Suspend = %v, want false", info.Suspend)
	}
	if info.ConcurrencyPolicy != "Forbid" {
		t.Errorf("ConcurrencyPolicy = %s, want Forbid", info.ConcurrencyPolicy)
	}
	if info.LastScheduleTime == nil {
		t.Error("LastScheduleTime should not be nil")
	}
	if info.ActiveJobs != 2 {
		t.Errorf("ActiveJobs = %d, want 2", info.ActiveJobs)
	}
}

func TestNewCronJobInfo_Defaults(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "default",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 * * * *",
		},
	}

	info := NewCronJobInfo(cj)

	if info.Suspend != false {
		t.Errorf("Suspend = %v, want false (default)", info.Suspend)
	}
	if info.ActiveJobs != 0 {
		t.Errorf("ActiveJobs = %d, want 0", info.ActiveJobs)
	}
	if info.LastScheduleTime != nil {
		t.Errorf("LastScheduleTime should be nil, got %v", info.LastScheduleTime)
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

func TestNewIngressInfo(t *testing.T) {
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "nginx"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			UID:       "ing-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-service",
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{"example.com"},
					SecretName: "tls-secret",
				},
			},
		},
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{IP: "192.168.1.1"},
				},
			},
		},
	}

	info := NewIngressInfo(ing)

	if info.UID != "ing-uid-123" {
		t.Errorf("UID = %s, want ing-uid-123", info.UID)
	}
	if info.Name != "test-ingress" {
		t.Errorf("Name = %s, want test-ingress", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.IngressClassName != "nginx" {
		t.Errorf("IngressClassName = %s, want nginx", info.IngressClassName)
	}
	if len(info.Rules) != 1 {
		t.Fatalf("Rules count = %d, want 1", len(info.Rules))
	}
	if info.Rules[0].Host != "example.com" {
		t.Errorf("Rules[0].Host = %s, want example.com", info.Rules[0].Host)
	}
	if len(info.Rules[0].Paths) != 1 {
		t.Fatalf("Rules[0].Paths count = %d, want 1", len(info.Rules[0].Paths))
	}
	if info.Rules[0].Paths[0].Path != "/api" {
		t.Errorf("Rules[0].Paths[0].Path = %s, want /api", info.Rules[0].Paths[0].Path)
	}
	if info.Rules[0].Paths[0].ServiceName != "api-service" {
		t.Errorf("Rules[0].Paths[0].ServiceName = %s, want api-service", info.Rules[0].Paths[0].ServiceName)
	}
	if info.Rules[0].Paths[0].ServicePort != 8080 {
		t.Errorf("Rules[0].Paths[0].ServicePort = %d, want 8080", info.Rules[0].Paths[0].ServicePort)
	}
	if len(info.TLS) != 1 {
		t.Fatalf("TLS count = %d, want 1", len(info.TLS))
	}
	if info.TLS[0].SecretName != "tls-secret" {
		t.Errorf("TLS[0].SecretName = %s, want tls-secret", info.TLS[0].SecretName)
	}
	if len(info.LoadBalancerIPs) != 1 {
		t.Fatalf("LoadBalancerIPs count = %d, want 1", len(info.LoadBalancerIPs))
	}
	if info.LoadBalancerIPs[0] != "192.168.1.1" {
		t.Errorf("LoadBalancerIPs[0] = %s, want 192.168.1.1", info.LoadBalancerIPs[0])
	}
}

func TestNewIngressInfo_Defaults(t *testing.T) {
	// Test with minimal ingress (no rules, no TLS, no ingress class)
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-ingress",
			Namespace: "default",
			UID:       "ing-uid-456",
		},
	}

	info := NewIngressInfo(ing)

	if info.IngressClassName != "" {
		t.Errorf("IngressClassName = %s, want empty", info.IngressClassName)
	}
	if len(info.Rules) != 0 {
		t.Errorf("Rules count = %d, want 0", len(info.Rules))
	}
	if len(info.TLS) != 0 {
		t.Errorf("TLS count = %d, want 0", len(info.TLS))
	}
	if len(info.LoadBalancerIPs) != 0 {
		t.Errorf("LoadBalancerIPs count = %d, want 0", len(info.LoadBalancerIPs))
	}
}

func TestNewNetworkPolicyInfo(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "default",
			UID:       "np-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "web",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{},
				{},
			},
		},
	}

	info := NewNetworkPolicyInfo(np)

	if info.UID != "np-uid-123" {
		t.Errorf("UID = %s, want np-uid-123", info.UID)
	}
	if info.Name != "test-netpol" {
		t.Errorf("Name = %s, want test-netpol", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.PodSelector["app"] != "web" {
		t.Errorf("PodSelector[app] = %s, want web", info.PodSelector["app"])
	}
	if len(info.PolicyTypes) != 2 {
		t.Fatalf("PolicyTypes count = %d, want 2", len(info.PolicyTypes))
	}
	if info.PolicyTypes[0] != "Ingress" {
		t.Errorf("PolicyTypes[0] = %s, want Ingress", info.PolicyTypes[0])
	}
	if info.PolicyTypes[1] != "Egress" {
		t.Errorf("PolicyTypes[1] = %s, want Egress", info.PolicyTypes[1])
	}
	if info.IngressRules != 1 {
		t.Errorf("IngressRules = %d, want 1", info.IngressRules)
	}
	if info.EgressRules != 2 {
		t.Errorf("EgressRules = %d, want 2", info.EgressRules)
	}
}

func TestNewNetworkPolicyInfo_Defaults(t *testing.T) {
	// Test with minimal network policy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-netpol",
			Namespace: "default",
			UID:       "np-uid-456",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
		},
	}

	info := NewNetworkPolicyInfo(np)

	if info.PodSelector != nil {
		t.Errorf("PodSelector = %v, want nil", info.PodSelector)
	}
	if len(info.PolicyTypes) != 0 {
		t.Errorf("PolicyTypes count = %d, want 0", len(info.PolicyTypes))
	}
	if info.IngressRules != 0 {
		t.Errorf("IngressRules = %d, want 0", info.IngressRules)
	}
	if info.EgressRules != 0 {
		t.Errorf("EgressRules = %d, want 0", info.EgressRules)
	}
}

func TestNewConfigMapInfo(t *testing.T) {
	immutable := true
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "default",
			UID:       "cm-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Data: map[string]string{
			"config.yaml":   "sensitive-value-should-not-be-included",
			"settings.json": "another-sensitive-value",
		},
		BinaryData: map[string][]byte{
			"binary.dat": []byte("binary-data"),
		},
		Immutable: &immutable,
	}

	info := NewConfigMapInfo(cm)

	if info.UID != "cm-uid-123" {
		t.Errorf("UID = %s, want cm-uid-123", info.UID)
	}
	if info.Name != "test-configmap" {
		t.Errorf("Name = %s, want test-configmap", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	// SECURITY: Verify we have keys but NOT values
	if len(info.DataKeys) != 2 {
		t.Errorf("DataKeys count = %d, want 2", len(info.DataKeys))
	}
	if len(info.BinaryKeys) != 1 {
		t.Errorf("BinaryKeys count = %d, want 1", len(info.BinaryKeys))
	}
	if !info.Immutable {
		t.Error("Immutable should be true")
	}

	// SECURITY: Verify values are NOT present in keys
	for _, key := range info.DataKeys {
		if key == "sensitive-value-should-not-be-included" {
			t.Fatalf("SECURITY VIOLATION: Data value found in DataKeys")
		}
	}
}

func TestNewConfigMapInfo_Defaults(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-configmap",
			Namespace: "default",
			UID:       "cm-uid-456",
		},
	}

	info := NewConfigMapInfo(cm)

	if info.DataKeys != nil {
		t.Errorf("DataKeys should be nil, got %v", info.DataKeys)
	}
	if info.BinaryKeys != nil {
		t.Errorf("BinaryKeys should be nil, got %v", info.BinaryKeys)
	}
	if info.Immutable {
		t.Error("Immutable should be false (default)")
	}
}

func TestNewSecretInfo(t *testing.T) {
	immutable := true
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			UID:       "secret-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("SENSITIVE-CERTIFICATE-DATA"),
			"tls.key": []byte("SENSITIVE-PRIVATE-KEY"),
		},
		Immutable: &immutable,
	}

	info := NewSecretInfo(secret)

	if info.UID != "secret-uid-123" {
		t.Errorf("UID = %s, want secret-uid-123", info.UID)
	}
	if info.Name != "test-secret" {
		t.Errorf("Name = %s, want test-secret", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.Type != "kubernetes.io/tls" {
		t.Errorf("Type = %s, want kubernetes.io/tls", info.Type)
	}
	// SECURITY: Verify we have keys but NOT values
	if len(info.DataKeys) != 2 {
		t.Errorf("DataKeys count = %d, want 2", len(info.DataKeys))
	}
	if !info.Immutable {
		t.Error("Immutable should be true")
	}

	// SECURITY CRITICAL: Verify sensitive values are NOT present
	for _, key := range info.DataKeys {
		if key == "SENSITIVE-CERTIFICATE-DATA" {
			t.Fatalf("SECURITY VIOLATION: Secret data value found in DataKeys")
		}
		if key == "SENSITIVE-PRIVATE-KEY" {
			t.Fatalf("SECURITY VIOLATION: Secret data value found in DataKeys")
		}
	}
}

func TestNewSecretInfo_Defaults(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-secret",
			Namespace: "default",
			UID:       "secret-uid-456",
		},
		Type: corev1.SecretTypeOpaque,
	}

	info := NewSecretInfo(secret)

	if info.DataKeys != nil {
		t.Errorf("DataKeys should be nil, got %v", info.DataKeys)
	}
	if info.Immutable {
		t.Error("Immutable should be false (default)")
	}
	if info.Type != "Opaque" {
		t.Errorf("Type = %s, want Opaque", info.Type)
	}
}

// SECURITY TEST: Verify no secret data values can ever be leaked
func TestNewSecretInfo_SecurityNoDataLeakage(t *testing.T) {
	// Create a secret with highly sensitive data
	sensitivePassword := "SUPER-SECRET-DATABASE-PASSWORD-12345"
	sensitiveKey := "PRIVATE-KEY-THAT-MUST-NEVER-LEAK"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sensitive-secret",
			Namespace: "production",
			UID:       "secret-uid-sensitive",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password":    []byte(sensitivePassword),
			"private-key": []byte(sensitiveKey),
		},
	}

	info := NewSecretInfo(secret)

	// Verify we only have key names
	if len(info.DataKeys) != 2 {
		t.Errorf("Expected 2 data keys, got %d", len(info.DataKeys))
	}

	// CRITICAL SECURITY CHECK: Ensure sensitive values are not present anywhere
	foundPassword := false
	foundKey := false
	for _, k := range info.DataKeys {
		if k == "password" {
			foundPassword = true
		}
		if k == "private-key" {
			foundKey = true
		}
		// These checks should NEVER fail if the code is correct
		if k == sensitivePassword {
			t.Fatalf("SECURITY VIOLATION: Sensitive password value found in DataKeys!")
		}
		if k == sensitiveKey {
			t.Fatalf("SECURITY VIOLATION: Sensitive private key value found in DataKeys!")
		}
	}

	if !foundPassword {
		t.Error("Expected 'password' key in DataKeys")
	}
	if !foundKey {
		t.Error("Expected 'private-key' key in DataKeys")
	}
}

func TestNewPVCInfo(t *testing.T) {
	storageClass := "fast-ssd"
	volumeMode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "pvc-uid-123",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany},
			VolumeMode:       &volumeMode,
			VolumeName:       "pv-123",
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	info := NewPVCInfo(pvc)

	if info.UID != "pvc-uid-123" {
		t.Errorf("UID = %s, want pvc-uid-123", info.UID)
	}
	if info.Name != "test-pvc" {
		t.Errorf("Name = %s, want test-pvc", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", info.Namespace)
	}
	if info.StorageClassName != "fast-ssd" {
		t.Errorf("StorageClassName = %s, want fast-ssd", info.StorageClassName)
	}
	if len(info.AccessModes) != 2 {
		t.Errorf("AccessModes count = %d, want 2", len(info.AccessModes))
	}
	if info.StorageRequest != "10Gi" {
		t.Errorf("StorageRequest = %s, want 10Gi", info.StorageRequest)
	}
	if info.VolumeName != "pv-123" {
		t.Errorf("VolumeName = %s, want pv-123", info.VolumeName)
	}
	if info.Phase != "Bound" {
		t.Errorf("Phase = %s, want Bound", info.Phase)
	}
	if info.VolumeMode != "Filesystem" {
		t.Errorf("VolumeMode = %s, want Filesystem", info.VolumeMode)
	}
}

func TestNewPVCInfo_Defaults(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-pvc",
			Namespace: "default",
			UID:       "pvc-uid-456",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	info := NewPVCInfo(pvc)

	if info.StorageClassName != "" {
		t.Errorf("StorageClassName = %s, want empty", info.StorageClassName)
	}
	if len(info.AccessModes) != 0 {
		t.Errorf("AccessModes count = %d, want 0", len(info.AccessModes))
	}
	if info.StorageRequest != "" {
		t.Errorf("StorageRequest = %s, want empty", info.StorageRequest)
	}
	if info.VolumeName != "" {
		t.Errorf("VolumeName = %s, want empty", info.VolumeName)
	}
	if info.Phase != "Pending" {
		t.Errorf("Phase = %s, want Pending", info.Phase)
	}
	if info.VolumeMode != "" {
		t.Errorf("VolumeMode = %s, want empty", info.VolumeMode)
	}
}
