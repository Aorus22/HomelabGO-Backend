package docker

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
)

const cloudflaredImage = "cloudflare/cloudflared:latest"

type CloudflaredStatus struct {
	ContainerID string `json:"container_id"`
	Status      string `json:"status"`
	State       string `json:"state"`
	Running     bool   `json:"running"`
}

func getCloudflaredContainerName(userID uint) string {
	return fmt.Sprintf("cloudflared_user_%d", userID)
}

func (c *Client) DeployCloudflared(ctx context.Context, userID uint, tunnelToken string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	containerName := getCloudflaredContainerName(userID)

	// Remove existing container if it exists
	existing, err := c.findContainerByName(ctx, containerName)
	if err == nil && existing != "" {
		_ = c.api.ContainerStop(ctx, existing, container.StopOptions{})
		_ = c.api.ContainerRemove(ctx, existing, container.RemoveOptions{Force: true})
	}

	// Pull latest cloudflared image
	_, err = c.api.ImagePull(ctx, cloudflaredImage, image.PullOptions{})
	if err != nil {
	}

	// Ensure user network exists
	networkID, err := c.EnsureUserNetwork(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to ensure user network: %w", err)
	}

	config := &container.Config{
		Image: cloudflaredImage,
		Cmd:   []string{"tunnel", "--no-autoupdate", "run", "--token", tunnelToken},
		Labels: map[string]string{
			"owner_id":   fmt.Sprintf("%d", userID),
			"managed_by": "homelabgo",
			"service":    "cloudflared",
		},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		NetworkMode:   container.NetworkMode(fmt.Sprintf("net_user_%d", userID)),
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			fmt.Sprintf("net_user_%d", userID): {
				NetworkID: networkID,
			},
		},
	}

	resp, err := c.api.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create cloudflared container: %w", err)
	}

	if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start cloudflared container: %w", err)
	}

	return nil
}

func (c *Client) GetCloudflaredStatus(ctx context.Context, userID uint) (*CloudflaredStatus, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	containerName := getCloudflaredContainerName(userID)

	args := filters.NewArgs()
	args.Add("name", containerName)

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return nil, err
	}

	for _, ctr := range containers {
		for _, name := range ctr.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return &CloudflaredStatus{
					ContainerID: ctr.ID[:12],
					Status:      ctr.Status,
					State:       ctr.State,
					Running:     ctr.State == "running",
				}, nil
			}
		}
	}

	return &CloudflaredStatus{
		Running: false,
		Status:  "not deployed",
		State:   "none",
	}, nil
}

func (c *Client) GetCloudflaredLogs(ctx context.Context, userID uint, tail string) (string, error) {
	if c.api == nil {
		return "", fmt.Errorf("docker client is not initialized")
	}

	containerName := getCloudflaredContainerName(userID)

	// Find container
	containerID, err := c.findContainerByName(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("cloudflared container not found")
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: true,
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

func (c *Client) StopCloudflared(ctx context.Context, userID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	containerName := getCloudflaredContainerName(userID)

	containerID, err := c.findContainerByName(ctx, containerName)
	if err != nil {
		return nil
	}

	_ = c.api.ContainerStop(ctx, containerID, container.StopOptions{})
	return c.api.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}
