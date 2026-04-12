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
	Labels  map[string]string // merged: container labels + image labels
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

		// Merge container labels + image labels
		// Container labels take priority over image labels
		mergedLabels := c.mergeLabels(ctx, ct.ID, ct.Labels)

		result = append(result, ContainerInfo{
			ID:      ct.ID[:12],
			Name:    name,
			Image:   ct.Image,
			ImageID: ct.ImageID,
			Status:  ct.Status,
			Running: ct.State == "running",
			Labels:  mergedLabels,
		})
	}

	logger.Log.Debugf("Found %d containers", len(result))
	return result, nil
}

// mergeLabels merges image labels (base) with container labels (override)
// Container labels always win over image labels
func (c *Client) mergeLabels(ctx context.Context, containerID string, containerLabels map[string]string) map[string]string {
	merged := make(map[string]string)

	// First inspect the image to get image-level labels
	inspect, err := c.CLI.ContainerInspect(ctx, containerID)
	if err != nil {
		logger.Log.Debugf("Could not inspect container %s for image labels: %v", containerID[:12], err)
		// Fall back to container labels only
		for k, v := range containerLabels {
			merged[k] = v
		}
		return merged
	}

	// Layer 1: image labels (lowest priority)
	if inspect.Config != nil {
		for k, v := range inspect.Config.Labels {
			merged[k] = v
		}
	}

	// Layer 2: container labels (highest priority, overrides image labels)
	for k, v := range containerLabels {
		merged[k] = v
	}

	return merged
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

		mergedLabels := c.mergeLabels(ctx, ct.ID, ct.Labels)

		result = append(result, ContainerInfo{
			ID:      ct.ID[:12],
			Name:    name,
			Image:   ct.Image,
			ImageID: ct.ImageID,
			Status:  ct.Status,
			Running: ct.State == "running",
			Labels:  mergedLabels,
		})
	}

	return result, nil
}