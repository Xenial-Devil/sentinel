package docker

import (
	"sentinel/config"
	"sentinel/logger"

	dockerclient "github.com/docker/docker/client"
)

// Client wraps the Docker client
type Client struct {
	CLI *dockerclient.Client
}

// New creates a new Docker client
func New(cfg *config.Config) (*Client, error) {
	var cli *dockerclient.Client
	var err error

	if cfg.DockerHost != "" {
		cli, err = dockerclient.NewClientWithOpts(
			dockerclient.WithHost(cfg.DockerHost),
			dockerclient.WithAPIVersionNegotiation(),
		)
	} else {
		cli, err = dockerclient.NewClientWithOpts(
			dockerclient.FromEnv,
			dockerclient.WithAPIVersionNegotiation(),
		)
	}

	if err != nil {
		return nil, err
	}

	logger.Log.Info("Connected to Docker daemon")
	return &Client{CLI: cli}, nil
}

// Close closes the Docker client
func (c *Client) Close() {
	// error is intentionally ignored on close - nothing actionable to do
	if err := c.CLI.Close(); err != nil {
		logger.Log.Warnf("Docker client close error: %v", err)
	}
	logger.Log.Info("Docker client closed")
}