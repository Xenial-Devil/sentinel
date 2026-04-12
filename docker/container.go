package docker

import (
	"context"
	"sentinel/logger"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// ContainerInfo holds container details
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	ImageID string
	Status  string
	Running bool
	Labels  map[string]string
}

// ListContainers returns all running containers
func (c *Client) ListContainers(includeStopped bool) ([]ContainerInfo, error) {
	ctx := context.Background()

	options := types.ContainerListOptions{
		All: includeStopped,
	}

	containers, err := c.CLI.ContainerList(ctx, options)
	if err != nil {
		logger.Log.Errorf("Failed to list containers: %v", err)
		return nil, err
	}

	var result []ContainerInfo
	for _, ct := range containers {
		name := "unknown"
		if len(ct.Names) > 0 {
			name = ct.Names[0][1:]
		}

		result = append(result, ContainerInfo{
			ID:      ct.ID[:12],
			Name:    name,
			Image:   ct.Image,
			ImageID: ct.ImageID,
			Status:  ct.Status,
			Running: ct.State == "running",
			Labels:  ct.Labels,
		})
	}

	logger.Log.Debugf("Found %d containers", len(result))
	return result, nil
}

// InspectContainer returns detailed info about a container
func (c *Client) InspectContainer(id string) (types.ContainerJSON, error) {
	ctx := context.Background()

	info, err := c.CLI.ContainerInspect(ctx, id)
	if err != nil {
		logger.Log.Errorf("Failed to inspect container %s: %v", id, err)
		return types.ContainerJSON{}, err
	}

	return info, nil
}

// StopContainer stops a running container
func (c *Client) StopContainer(id string, timeout int) error {
	ctx := context.Background()

	stopTimeout := timeout
	err := c.CLI.ContainerStop(ctx, id, container.StopOptions{
		Timeout: &stopTimeout,
	})
	if err != nil {
		logger.Log.Errorf("Failed to stop container %s: %v", id, err)
		return err
	}

	logger.Log.Infof("Container stopped: %s", id)
	return nil
}

// RemoveContainer removes a stopped container
func (c *Client) RemoveContainer(id string) error {
	ctx := context.Background()

	err := c.CLI.ContainerRemove(ctx, id, types.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		logger.Log.Errorf("Failed to remove container %s: %v", id, err)
		return err
	}

	logger.Log.Infof("Container removed: %s", id)
	return nil
}

// StartContainer starts a stopped container
func (c *Client) StartContainer(id string) error {
	ctx := context.Background()

	err := c.CLI.ContainerStart(ctx, id, types.ContainerStartOptions{})
	if err != nil {
		logger.Log.Errorf("Failed to start container %s: %v", id, err)
		return err
	}

	logger.Log.Infof("Container started: %s", id)
	return nil
}

// GetContainersByLabel filters containers by label
func (c *Client) GetContainersByLabel(labelKey string, labelValue string) ([]ContainerInfo, error) {
	ctx := context.Background()

	f := filters.NewArgs()
	f.Add("label", labelKey+"="+labelValue)

	options := types.ContainerListOptions{
		Filters: f,
	}

	containers, err := c.CLI.ContainerList(ctx, options)
	if err != nil {
		logger.Log.Errorf("Failed to filter containers: %v", err)
		return nil, err
	}

	var result []ContainerInfo
	for _, ct := range containers {
		name := "unknown"
		if len(ct.Names) > 0 {
			name = ct.Names[0][1:]
		}

		result = append(result, ContainerInfo{
			ID:      ct.ID[:12],
			Name:    name,
			Image:   ct.Image,
			ImageID: ct.ImageID,
			Status:  ct.Status,
			Running: ct.State == "running",
			Labels:  ct.Labels,
		})
	}

	return result, nil
}