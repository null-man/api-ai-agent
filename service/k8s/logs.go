package k8s

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodInfo struct {
	Name         string `json:"name"`
	Phase        string `json:"phase"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Container    string `json:"container"`
}

type CapacityInfo struct {
	NodeName                string `json:"node_name"`
	AllocatableMemoryBytes  int64  `json:"allocatable_memory_bytes"`
	RequestedMemoryBytes    int64  `json:"requested_memory_bytes"`
	RemainingMemoryBytes    int64  `json:"remaining_memory_bytes"`
	AllocatableCPUMilli     int64  `json:"allocatable_cpu_milli"`
	RequestedCPUMilli       int64  `json:"requested_cpu_milli"`
	RemainingCPUMilli       int64  `json:"remaining_cpu_milli"`
	BotRequestMemoryBytes   int64  `json:"bot_request_memory_bytes"`
	BotRequestCPUMilli      int64  `json:"bot_request_cpu_milli"`
	RunningBotCount         int    `json:"running_bot_count"`
	EstimatedAdditionalBots int    `json:"estimated_additional_bots"`
}

func GetLatestPodInfo(ctx context.Context, botID string) (*PodInfo, error) {
	client := GetClient()
	namespace := GetNamespace()

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=openclaw,bot-id=%s", botID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return nil, nil
	}

	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.Time.After(pods.Items[j].CreationTimestamp.Time)
	})

	pod := pods.Items[0]
	info := &PodInfo{
		Name:  pod.Name,
		Phase: string(pod.Status.Phase),
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "openclaw" {
			info.Container = cs.Name
			info.Ready = cs.Ready
			info.RestartCount = cs.RestartCount
			break
		}
	}

	if info.Container == "" && len(pod.Status.ContainerStatuses) > 0 {
		cs := pod.Status.ContainerStatuses[0]
		info.Container = cs.Name
		info.Ready = cs.Ready
		info.RestartCount = cs.RestartCount
	}

	return info, nil
}

func GetPodLogs(ctx context.Context, podName, container string, tailLines int64) (string, error) {
	client := GetClient()
	namespace := GetNamespace()

	if container == "" {
		container = "openclaw"
	}
	if tailLines <= 0 {
		tailLines = 200
	}

	req := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailLines,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to stream pod logs: %w", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("failed to read pod logs: %w", err)
	}

	return string(data), nil
}

func GetCapacityInfo(ctx context.Context) (*CapacityInfo, error) {
	client := GetClient()
	namespace := GetNamespace()

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes found")
	}

	// Current deployment is single-node, so estimate against the first schedulable node.
	node := nodes.Items[0]
	nodeName := node.Name

	allocMem := node.Status.Allocatable.Memory().Value()
	allocCPU := node.Status.Allocatable.Cpu().MilliValue()

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var requestedMem int64
	var requestedCPU int64
	runningBots := 0

	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Spec.NodeName != nodeName {
			continue
		}

		if pod.Labels["app"] == "openclaw" && pod.Status.Phase == corev1.PodRunning {
			runningBots++
		}

		for _, container := range pod.Spec.Containers {
			if req := container.Resources.Requests.Memory(); req != nil {
				requestedMem += req.Value()
			}
			if req := container.Resources.Requests.Cpu(); req != nil {
				requestedCPU += req.MilliValue()
			}
		}
	}

	botMemQty := resource.MustParse(defaultString(viper.GetString("openclaw.memory_request"), "512Mi"))
	botCPUQty := resource.MustParse(defaultString(viper.GetString("openclaw.cpu_request"), "250m"))
	botMemReq := (&botMemQty).Value()
	botCPUReq := (&botCPUQty).MilliValue()

	remainingMem := allocMem - requestedMem
	if remainingMem < 0 {
		remainingMem = 0
	}
	remainingCPU := allocCPU - requestedCPU
	if remainingCPU < 0 {
		remainingCPU = 0
	}

	memBots := int64(0)
	if botMemReq > 0 {
		memBots = remainingMem / botMemReq
	}
	cpuBots := int64(0)
	if botCPUReq > 0 {
		cpuBots = remainingCPU / botCPUReq
	}

	estimated := memBots
	if cpuBots < estimated || estimated == 0 {
		estimated = cpuBots
	}
	if botMemReq > 0 && cpuBots == 0 {
		estimated = memBots
	}
	if estimated < 0 {
		estimated = 0
	}

	return &CapacityInfo{
		NodeName:                nodeName,
		AllocatableMemoryBytes:  allocMem,
		RequestedMemoryBytes:    requestedMem,
		RemainingMemoryBytes:    remainingMem,
		AllocatableCPUMilli:     allocCPU,
		RequestedCPUMilli:       requestedCPU,
		RemainingCPUMilli:       remainingCPU,
		BotRequestMemoryBytes:   botMemReq,
		BotRequestCPUMilli:      botCPUReq,
		RunningBotCount:         runningBots,
		EstimatedAdditionalBots: int(estimated),
	}, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
