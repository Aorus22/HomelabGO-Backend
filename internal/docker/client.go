package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type Client struct {
	api *client.Client
}

func NewClient() (*Client, error) {
	api, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &Client{api: api}, nil
}

func (c *Client) Close() error {
	if c.api == nil {
		return nil
	}
	return c.api.Close()
}

// GetAPI returns the underlying Docker API client for advanced operations
func (c *Client) GetAPI() *client.Client {
	return c.api
}

// ListContainers returns all containers for a user (alias for ListContainersByUser)
func (c *Client) ListContainers(ctx context.Context, userID uint) ([]ContainerInfo, error) {
	return c.ListContainersByUser(ctx, userID)
}

// PullImage pulls a Docker image
func (c *Client) PullImage(ctx context.Context, imageName string) (io.ReadCloser, error) {
	return c.api.ImagePull(ctx, imageName, image.PullOptions{})
}

// StopContainerByID stops a container without ownership check (admin use)
func (c *Client) StopContainerByID(ctx context.Context, containerID string) error {
	timeout := 10
	return c.api.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// RemoveContainer removes a container without ownership check (admin use)
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.api.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// GetContainerByIDNoAuth gets container info without ownership check (admin use)
func (c *Client) GetContainerByIDNoAuth(ctx context.Context, containerID string) (*ContainerInfo, error) {
	args := filters.NewArgs()
	args.Add("id", containerID)

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil || len(containers) == 0 {
		return nil, err
	}

	ctr := containers[0]
	return &ContainerInfo{
		ID:          ctr.ID[:12],
		Name:        ctr.Names[0],
		Image:       ctr.Image,
		Status:      ctr.Status,
		State:       ctr.State,
		ProjectName: ctr.Labels["project_name"],
		ServiceName: ctr.Labels["service_name"],
		Labels:      ctr.Labels,
	}, nil
}
