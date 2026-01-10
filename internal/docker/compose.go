package docker

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/goccy/go-yaml"
)

type ComposeService struct {
	Image         string            `yaml:"image"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Command       string            `yaml:"command,omitempty"`
	Restart       string            `yaml:"restart,omitempty"`
	XHlgoEnvfiles []int             `yaml:"x-hlgo-envfiles,omitempty"`
}

type ComposeFile struct {
	Version  string                    `yaml:"version,omitempty"`
	Services map[string]ComposeService `yaml:"services"`
}

func ParseComposeYAML(yamlContent string) (*ComposeFile, error) {
	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(yamlContent), &compose); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("no services defined")
	}

	return &compose, nil
}

func ValidateComposeYAML(yamlContent string) error {
	compose, err := ParseComposeYAML(yamlContent)
	if err != nil {
		return err
	}

	for name, service := range compose.Services {
		if service.Image == "" {
			return fmt.Errorf("service '%s': image is required", name)
		}

		for _, port := range service.Ports {
			if !isValidPortMapping(port) {
				return fmt.Errorf("service '%s': invalid port mapping '%s'", name, port)
			}
		}
	}

	return nil
}

func isValidPortMapping(port string) bool {
	parts := strings.Split(port, ":")
	return len(parts) >= 1 && len(parts) <= 3
}

type DeployedContainer struct {
	ID          string
	Name        string
	ServiceName string
}

// envFileContentMap: map[envFileID] => content (KEY=VALUE lines)
func (c *Client) DeployCompose(ctx context.Context, userID uint, projectID uint, projectName string, yamlContent string, envFileContentMap map[int]string) ([]DeployedContainer, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	compose, err := ParseComposeYAML(yamlContent)
	if err != nil {
		return nil, err
	}

	// Ensure user network exists
	networkID, err := c.EnsureUserNetwork(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure user network: %w", err)
	}

	// Snapshot old images
	oldImageIDs := make(map[string]bool)
	existingContainers, _ := c.GetProjectContainers(ctx, userID, projectID)
	for _, ctr := range existingContainers {
		if json, err := c.api.ContainerInspect(ctx, ctr.ID); err == nil {
			oldImageIDs[json.Image] = true
		}
	}

	deployed := make([]DeployedContainer, 0, len(compose.Services))

	for serviceName, service := range compose.Services {
		containerName := fmt.Sprintf("%s_%s_user_%d", projectName, serviceName, userID)

		// Check if container already exists
		existing, err := c.findContainerByName(ctx, containerName)
		if err == nil && existing != "" {
			// Remove existing container
			_ = c.api.ContainerRemove(ctx, existing, container.RemoveOptions{Force: true})
		}

		// Ensure image exists (Pull)
		reader, err := c.api.ImagePull(ctx, service.Image, image.PullOptions{})
		if err != nil {
			// Try to pull, if fail, check if we have it locally
			_, _, inspectErr := c.api.ImageInspectWithRaw(ctx, service.Image)
			if inspectErr != nil {
				return nil, fmt.Errorf("failed to pull image '%s': %w", service.Image, err)
			}
		} else {
			defer reader.Close()
			_, _ = io.Copy(io.Discard, reader)
		}

		// Build env: first from compose, then from env files (env files take precedence)
		env := make([]string, 0, len(service.Environment))
		for k, v := range service.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		// Append env from env files specified in x-hlgo-envfiles
		for _, efID := range service.XHlgoEnvfiles {
			if content, ok := envFileContentMap[efID]; ok {
				// Parse KEY=VALUE lines
				for _, line := range strings.Split(content, "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					env = append(env, line)
				}
			}
		}

		mounts := make([]mount.Mount, 0, len(service.Volumes))
		for _, v := range service.Volumes {
			parts := strings.Split(v, ":")
			if len(parts) >= 2 {
				// Replace volume name with user-specific volume
				sourceName := fmt.Sprintf("vol_user_%d_%s", userID, parts[0])
				// Auto-create volume if not exists
				_, _ = c.CreateVolume(ctx, userID, parts[0])

				mounts = append(mounts, mount.Mount{
					Type:   mount.TypeVolume,
					Source: sourceName,
					Target: parts[1],
				})
			}
		}

		config := &container.Config{
			Image: service.Image,
			Env:   env,
			Labels: map[string]string{
				"owner_id":     fmt.Sprintf("%d", userID),
				"project_id":   fmt.Sprintf("%d", projectID),
				"project_name": projectName,
				"service_name": serviceName,
				"managed_by":   "homelabgo",
			},
		}

		if service.Command != "" {
			config.Cmd = strings.Fields(service.Command)
		}

		hostConfig := &container.HostConfig{
			Mounts:      mounts,
			NetworkMode: container.NetworkMode(fmt.Sprintf("net_user_%d", userID)),
		}

		if service.Restart != "" {
			hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(service.Restart)}
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
			return deployed, fmt.Errorf("failed to create container '%s': %w", serviceName, err)
		}

		if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return deployed, fmt.Errorf("failed to start container '%s': %w", serviceName, err)
		}

		deployed = append(deployed, DeployedContainer{
			ID:          resp.ID,
			Name:        containerName,
			ServiceName: serviceName,
		})
	}

	// Prune old images
	for imgID := range oldImageIDs {
		if _, err := c.api.ImageRemove(ctx, imgID, image.RemoveOptions{PruneChildren: true}); err == nil {
			fmt.Printf("Pruned old image: %s\n", imgID)
		}
	}

	return deployed, nil
}

func (c *Client) GetProjectContainers(ctx context.Context, userID uint, projectID uint) ([]container.Summary, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("owner_id=%d", userID))
	args.Add("label", fmt.Sprintf("project_id=%d", projectID))
	args.Add("label", "managed_by=homelabgo")

	return c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
}

func (c *Client) StopProjectContainers(ctx context.Context, userID uint, projectID uint) error {
	containers, err := c.GetProjectContainers(ctx, userID, projectID)
	if err != nil {
		return err
	}

	for _, ctr := range containers {
		_ = c.api.ContainerStop(ctx, ctr.ID, container.StopOptions{})
	}
	return nil
}

func (c *Client) StartProjectContainers(ctx context.Context, userID uint, projectID uint) error {
	containers, err := c.GetProjectContainers(ctx, userID, projectID)
	if err != nil {
		return err
	}

	for _, ctr := range containers {
		_ = c.api.ContainerStart(ctx, ctr.ID, container.StartOptions{})
	}
	return nil
}

func (c *Client) SmartStartCompose(ctx context.Context, userID uint, projectID uint, projectName string, yamlContent string, envFileContentMap map[int]string) ([]DeployedContainer, bool, error) {
	// 1. Parse YAML to see desired state
	compose, err := ParseComposeYAML(yamlContent)
	if err != nil {
		return nil, false, err
	}

	// 2. Get existing containers
	containers, err := c.GetProjectContainers(ctx, userID, projectID)
	if err != nil {
		return nil, false, err
	}

	// 3. Check if all services exist and have correct image (by name check)
	// We map service_name -> container
	existingServices := make(map[string]container.Summary)
	for _, ctr := range containers {
		if serviceName, ok := ctr.Labels["service_name"]; ok {
			existingServices[serviceName] = ctr
		}
	}

	needsRedeploy := false
	if len(containers) == 0 {
		needsRedeploy = true
	} else {
		for serviceName, service := range compose.Services {
			ctr, exists := existingServices[serviceName]
			if !exists {
				needsRedeploy = true
				break
			}

			// Check if image changed
			if ctr.Image != service.Image {
				needsRedeploy = true
				break
			}
		}
	}

	if needsRedeploy {
		// Call DeployCompose (which pulls, creates, prunes)
		deployed, err := c.DeployCompose(ctx, userID, projectID, projectName, yamlContent, envFileContentMap)
		return deployed, true, err
	} else {
		// Just start existing
		err := c.StartProjectContainers(ctx, userID, projectID)
		return nil, false, err
	}
}

func (c *Client) findContainerByName(ctx context.Context, name string) (string, error) {
	args := filters.NewArgs()
	args.Add("name", name)

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return "", err
	}

	for _, ctr := range containers {
		for _, n := range ctr.Names {
			if strings.TrimPrefix(n, "/") == name {
				return ctr.ID, nil
			}
		}
	}

	return "", fmt.Errorf("container not found")
}

func (c *Client) RemoveProjectContainers(ctx context.Context, userID uint, projectID uint) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("owner_id=%d", userID))
	args.Add("label", fmt.Sprintf("project_id=%d", projectID))
	args.Add("label", "managed_by=homelabgo")

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return err
	}

	for _, ctr := range containers {
		_ = c.api.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true})
	}

	return nil
}
