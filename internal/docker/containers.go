package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
)

type ContainerInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Status      string            `json:"status"`
	State       string            `json:"state"`
	Created     time.Time         `json:"created"`
	ProjectName string            `json:"project_name"`
	ServiceName string            `json:"service_name"`
	Labels      map[string]string `json:"labels"`
}

func (c *Client) ListContainersByUser(ctx context.Context, userID uint) ([]ContainerInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("owner_id=%d", userID))
	args.Add("label", "managed_by=homelabgo")

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return nil, err
	}

	result := make([]ContainerInfo, len(containers))
	for i, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		result[i] = ContainerInfo{
			ID:          ctr.ID[:12],
			Name:        name,
			Image:       ctr.Image,
			Status:      ctr.Status,
			State:       ctr.State,
			Created:     time.Unix(ctr.Created, 0),
			ProjectName: ctr.Labels["project_name"],
			ServiceName: ctr.Labels["service_name"],
			Labels:      ctr.Labels,
		}
	}

	return result, nil
}

func (c *Client) GetContainerByID(ctx context.Context, containerID string, userID uint) (*ContainerInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	inspect, err := c.api.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("container not found")
	}

	// Verify ownership (bypass if userID is 0, used for admin)
	if userID != 0 {
		ownerID := inspect.Config.Labels["owner_id"]
		if ownerID != fmt.Sprintf("%d", userID) {
			return nil, fmt.Errorf("access denied")
		}
	}

	createdTime, _ := time.Parse(time.RFC3339Nano, inspect.Created)

	return &ContainerInfo{
		ID:          inspect.ID[:12],
		Name:        strings.TrimPrefix(inspect.Name, "/"),
		Image:       inspect.Config.Image,
		Status:      inspect.State.Status,
		State:       inspect.State.Status,
		Created:     createdTime,
		ProjectName: inspect.Config.Labels["project_name"],
		ServiceName: inspect.Config.Labels["service_name"],
		Labels:      inspect.Config.Labels,
	}, nil
}

func (c *Client) StartContainer(ctx context.Context, containerID string, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	return c.api.ContainerStart(ctx, containerID, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, containerID string, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	timeout := 10
	return c.api.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func (c *Client) RestartContainer(ctx context.Context, containerID string, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	timeout := 10
	return c.api.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func (c *Client) GetContainerLogs(ctx context.Context, containerID string, userID uint, tail string, since string) (string, error) {
	if c.api == nil {
		return "", fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return "", err
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: true,
	}

	if since != "" {
		options.Since = since
	}

	reader, err := c.api.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	logs, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return stripDockerLogHeaders(string(logs)), nil
}

func stripDockerLogHeaders(logs string) string {
	lines := strings.Split(logs, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		if len(line) > 8 {
			result = append(result, line[8:])
		} else if len(line) > 0 {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func (c *Client) StreamContainerLogs(ctx context.Context, containerID string, userID uint) (io.ReadCloser, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return nil, err
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "100",
		Timestamps: true,
	}

	return c.api.ContainerLogs(ctx, containerID, options)
}

func (c *Client) ExecContainer(ctx context.Context, containerID string, userID uint, shell string) (types.HijackedResponse, string, error) {
	if c.api == nil {
		return types.HijackedResponse{}, "", fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return types.HijackedResponse{}, "", err
	}

	cmd := []string{"/bin/sh"}
	if shell != "" {
		cmd = []string{shell}
	}

	execConfig := container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		// Only retry with bash if no shell was specified and sh failed
		if shell == "" {
			execConfig.Cmd = []string{"/bin/bash"}
			execID, err = c.api.ContainerExecCreate(ctx, containerID, execConfig)
			if err != nil {
				return types.HijackedResponse{}, "", fmt.Errorf("failed to create exec: %w", err)
			}
		} else {
			return types.HijackedResponse{}, "", fmt.Errorf("failed to create exec with shell %s: %w", shell, err)
		}
	}

	resp, err := c.api.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{
		Tty: true,
	})
	if err != nil {
		return types.HijackedResponse{}, "", fmt.Errorf("failed to attach to exec: %w", err)
	}

	return resp, execID.ID, nil
}

func (c *Client) PullContainerImage(ctx context.Context, containerID string, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	ctr, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	reader, err := c.api.ImagePull(ctx, ctr.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull output: %w", err)
	}

	return nil
}

func (c *Client) RecreateContainer(ctx context.Context, containerID string, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	inspect, err := c.api.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	timeout := 10
	_ = c.api.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err := c.api.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove old container: %w", err)
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: inspect.NetworkSettings.Networks,
	}

	resp, err := c.api.ContainerCreate(ctx, inspect.Config, inspect.HostConfig, networkingConfig, nil, strings.TrimPrefix(inspect.Name, "/"))
	if err != nil {
		return fmt.Errorf("failed to create new container: %w", err)
	}

	// Start new container
	if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start new container: %w", err)
	}

	// Prune old image
	if inspect.Image != "" {
		if _, err := c.api.ImageRemove(ctx, inspect.Image, image.RemoveOptions{PruneChildren: true}); err == nil {
			fmt.Printf("Pruned old image: %s\n", inspect.Image)
		} else {
			// Debug log
			fmt.Printf("Failed to prune old image %s: %v\n", inspect.Image, err)
		}
	}

	return nil
}

type ContainerStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkRx     uint64  `json:"network_rx"`
	NetworkTx     uint64  `json:"network_tx"`
}

func (c *Client) GetContainerStats(ctx context.Context, containerID string, userID uint) (*ContainerStats, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return nil, err
	}

	stats, err := c.api.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()

	var statJSON struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage  uint64   `json:"total_usage"`
				PercpuUsage []uint64 `json:"percpu_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
	}
	if err := json.NewDecoder(stats.Body).Decode(&statJSON); err != nil {
		return nil, err
	}

	cpuDelta := float64(statJSON.CPUStats.CPUUsage.TotalUsage - statJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statJSON.CPUStats.SystemUsage - statJSON.PreCPUStats.SystemUsage)
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(statJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	memoryPercent := 0.0
	if statJSON.MemoryStats.Limit > 0 {
		memoryPercent = (float64(statJSON.MemoryStats.Usage) / float64(statJSON.MemoryStats.Limit)) * 100.0
	}

	var networkRx, networkTx uint64
	for _, netStats := range statJSON.Networks {
		networkRx += netStats.RxBytes
		networkTx += netStats.TxBytes
	}

	return &ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryUsage:   statJSON.MemoryStats.Usage,
		MemoryLimit:   statJSON.MemoryStats.Limit,
		MemoryPercent: memoryPercent,
		NetworkRx:     networkRx,
		NetworkTx:     networkTx,
	}, nil
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

func (c *Client) GetContainerMounts(ctx context.Context, containerID string, userID uint) ([]MountInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return nil, err
	}

	inspect, err := c.api.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	mounts := make([]MountInfo, len(inspect.Mounts))
	for i, mount := range inspect.Mounts {
		mounts[i] = MountInfo{
			Type:        string(mount.Type),
			Source:      mount.Source,
			Destination: mount.Destination,
			Mode:        mount.Mode,
			RW:          mount.RW,
		}
	}

	return mounts, nil
}
