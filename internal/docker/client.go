package docker

import "github.com/docker/docker/client"

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
