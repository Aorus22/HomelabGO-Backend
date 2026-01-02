package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
)

type VolumeInfo struct {
	Name      string
	MountPath string
	CreatedAt string
}

func (c *Client) CreateVolume(ctx context.Context, userID uint, volumeName string) (*VolumeInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	fullName := fmt.Sprintf("vol_user_%d_%s", userID, volumeName)

	vol, err := c.api.VolumeCreate(ctx, volume.CreateOptions{
		Name: fullName,
		Labels: map[string]string{
			"owner_id":    fmt.Sprintf("%d", userID),
			"volume_name": volumeName,
			"managed_by":  "homelabgo",
		},
	})
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		Name:      vol.Name,
		MountPath: vol.Mountpoint,
		CreatedAt: vol.CreatedAt,
	}, nil
}

func (c *Client) ListVolumesByUser(ctx context.Context, userID uint) ([]VolumeInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("owner_id=%d", userID))
	args.Add("label", "managed_by=homelabgo")

	resp, err := c.api.VolumeList(ctx, volume.ListOptions{Filters: args})
	if err != nil {
		return nil, err
	}

	volumes := make([]VolumeInfo, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		volumes = append(volumes, VolumeInfo{
			Name:      v.Name,
			MountPath: v.Mountpoint,
			CreatedAt: v.CreatedAt,
		})
	}

	return volumes, nil
}

func (c *Client) DeleteVolume(ctx context.Context, userID uint, volumeName string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	fullName := fmt.Sprintf("vol_user_%d_%s", userID, volumeName)

	// Verify ownership by inspecting the volume
	vol, err := c.api.VolumeInspect(ctx, fullName)
	if err != nil {
		return fmt.Errorf("volume not found: %w", err)
	}

	ownerID := vol.Labels["owner_id"]
	if ownerID != fmt.Sprintf("%d", userID) {
		return fmt.Errorf("access denied: volume does not belong to user")
	}

	return c.api.VolumeRemove(ctx, fullName, false)
}

func (c *Client) GetVolume(ctx context.Context, userID uint, volumeName string) (*VolumeInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	fullName := fmt.Sprintf("vol_user_%d_%s", userID, volumeName)

	vol, err := c.api.VolumeInspect(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %w", err)
	}

	ownerID := vol.Labels["owner_id"]
	if ownerID != fmt.Sprintf("%d", userID) {
		return nil, fmt.Errorf("access denied: volume does not belong to user")
	}

	return &VolumeInfo{
		Name:      vol.Name,
		MountPath: vol.Mountpoint,
		CreatedAt: vol.CreatedAt,
	}, nil
}

func (c *Client) ensureImage(ctx context.Context, imageName string) error {
	_, _, err := c.api.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil
	}

	reader, err := c.api.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
	return nil
}

func (c *Client) DownloadVolumeAsTarGz(ctx context.Context, userID uint, volumeName string, mountPath string) ([]byte, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	// Ensure busybox image exists
	if err := c.ensureImage(ctx, "busybox"); err != nil {
		return nil, fmt.Errorf("failed to pull required image: %w", err)
	}

	fullName := fmt.Sprintf("vol_user_%d_%s", userID, volumeName)

	// Verify ownership
	vol, err := c.api.VolumeInspect(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %w", err)
	}

	ownerID := vol.Labels["owner_id"]
	if ownerID != fmt.Sprintf("%d", userID) {
		return nil, fmt.Errorf("access denied")
	}

	// Use a temporary container to tar the volume contents
	// We'll use busybox to create a tar.gz of the volume
	containerConfig := &container.Config{
		Image: "busybox",
		Cmd:   []string{"tar", "-czf", "-", "-C", "/data", "."},
	}

	hostConfig := &container.HostConfig{
		Binds:      []string{fullName + ":/data:ro"},
		AutoRemove: false,
	}

	// Create container
	resp, err := c.api.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create tar container: %w", err)
	}
	defer c.api.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Start container
	if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start tar container: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := c.api.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("failed waiting for container: %w", err)
		}
	case <-statusCh:
	}

	// Get logs (stdout contains the tar data)
	logs, err := c.api.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get tar data: %w", err)
	}
	defer logs.Close()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	// Use StdCopy to demultiplex the stream and get only stdout
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, logs)
	if err != nil {
		return nil, fmt.Errorf("failed to read tar data: %w", err)
	}

	if errBuf.Len() > 0 {
		return nil, fmt.Errorf("tar command failed: %s", errBuf.String())
	}

	return outBuf.Bytes(), nil
}

func (c *Client) CreateVolumeFromTarGz(ctx context.Context, userID uint, volumeName string, tarData []byte) (*VolumeInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	// Ensure busybox image exists
	if err := c.ensureImage(ctx, "busybox"); err != nil {
		return nil, fmt.Errorf("failed to pull required image: %w", err)
	}

	volInfo, err := c.CreateVolume(ctx, userID, volumeName)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("vol_user_%d_%s", userID, volumeName)

	containerConfig := &container.Config{
		Image:     "busybox",
		Cmd:       []string{"tar", "-xzf", "-", "-C", "/data"},
		Tty:       false,
		OpenStdin: true,
		StdinOnce: true,
	}

	hostConfig := &container.HostConfig{
		Binds:      []string{fullName + ":/data"},
		AutoRemove: true,
	}

	resp, err := c.api.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		c.DeleteVolume(ctx, userID, volumeName)
		return nil, fmt.Errorf("failed to create extract container: %w", err)
	}

	attachResp, err := c.api.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		c.api.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		c.DeleteVolume(ctx, userID, volumeName)
		return nil, fmt.Errorf("failed to attach to container: %w", err)
	}
	defer attachResp.Close()

	if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		c.api.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		c.DeleteVolume(ctx, userID, volumeName)
		return nil, fmt.Errorf("failed to start extract container: %w", err)
	}

	_, err = attachResp.Conn.Write(tarData)
	if err != nil {
		c.DeleteVolume(ctx, userID, volumeName)
		return nil, fmt.Errorf("failed to write tar data: %w", err)
	}
	attachResp.CloseWrite()

	statusCh, errCh := c.api.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			c.DeleteVolume(ctx, userID, volumeName)
			return nil, fmt.Errorf("failed waiting for container: %w", err)
		}
	case <-statusCh:
	}

	return volInfo, nil
}
