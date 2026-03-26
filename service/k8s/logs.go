package k8s

import (
	"context"
	"fmt"
	"io"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodInfo struct {
	Name         string `json:"name"`
	Phase        string `json:"phase"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Container    string `json:"container"`
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
