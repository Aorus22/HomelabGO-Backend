package docker

import (
	"context"
	"fmt"
	"strconv"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

func (c *Client) EnsureUserNetwork(ctx context.Context, userID uint) (string, error) {
	if c.api == nil {
		return "", fmt.Errorf("docker client is not initialized")
	}

	name := fmt.Sprintf("net_user_%d", userID)
	args := filters.NewArgs()
	args.Add("name", name)

	networks, err := c.api.NetworkList(ctx, network.ListOptions{Filters: args})
	if err != nil {
		return "", err
	}

	for _, network := range networks {
		if network.Name == name {
			return network.ID, nil
		}
	}

	resp, err := c.api.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:         "bridge",
		Labels: map[string]string{
			"owner_id": strconv.FormatUint(uint64(userID), 10),
		},
	})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}
