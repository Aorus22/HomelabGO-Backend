package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
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
