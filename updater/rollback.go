package updater

import (
	"context"
	"fmt"
	"sentinel/logger"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// RollbackState saves container state before update
type RollbackState struct {
	ContainerName string
	OldImageID    string
	OldImage      string
	Info          dockertypes.ContainerJSON
}

// SaveState saves current container state for rollback
func (u *Updater) SaveState(ct dockertypes.ContainerJSON) *RollbackState {
	return &RollbackState{
		ContainerName: ct.Name[1:],
		OldImageID:    ct.Image,
		OldImage:      ct.Config.Image,
		Info:          ct,
	}
}

// Rollback reverts a container to its previous state
func (u *Updater) Rollback(state *RollbackState) error {
	logger.LogRollbackStart(state.ContainerName, "", state.OldImage)

	// Stop broken container
	if err := u.stopCurrentContainer(state.ContainerName); err != nil {
		logger.Log.Warnf("Failed to stop container during rollback: %v", err)
	}

	// Remove broken container
	if err := u.removeCurrentContainer(state.ContainerName); err != nil {
		logger.Log.Warnf("Failed to remove container during rollback: %v", err)
	}

	// Restore old image
	state.Info.Config.Image = state.OldImage

	if err := u.recreateContainer(state.ContainerName, state.Info); err != nil {
		return fmt.Errorf("rollback recreate failed: %v", err)
	}

	logger.LogRollbackSuccess(state.ContainerName, state.OldImage)
	return nil
}

// WaitForHealthy waits for container to become healthy
func (u *Updater) WaitForHealthy(containerName string) error {
	logger.Log.Infof("Waiting for %s to become healthy...", containerName)

	timeout := time.Duration(u.Config.HealthTimeout) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := u.getHealthStatus(containerName)
		if err != nil {
			logger.Log.Warnf("Failed to get health status: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		switch status {
		case "healthy":
			logger.LogHealthCheck(containerName, true, time.Since(deadline.Add(-timeout)))
			return nil
		case "unhealthy":
			logger.LogHealthCheck(containerName, false, time.Since(deadline.Add(-timeout)))
			return fmt.Errorf("container %s is unhealthy", containerName)
		case "none":
			// No healthcheck - assume healthy after short wait
			time.Sleep(2 * time.Second)
			return nil
		default:
			logger.Log.Debugf("Container %s status: %s - waiting...", containerName, status)
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("health timeout after %ds for %s", u.Config.HealthTimeout, containerName)
}

// getHealthStatus returns health status of a container
func (u *Updater) getHealthStatus(containerName string) (string, error) {
	ctx := context.Background()
	info, err := u.Client.CLI.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}
	if info.State.Health == nil {
		return "none", nil
	}
	return info.State.Health.Status, nil
}

func (u *Updater) stopCurrentContainer(name string) error {
	ctx := context.Background()
	timeout := u.Config.StopTimeout
	return u.Client.CLI.ContainerStop(ctx, name, container.StopOptions{
		Timeout: &timeout,
	})
}

func (u *Updater) removeCurrentContainer(name string) error {
	ctx := context.Background()
	return u.Client.CLI.ContainerRemove(ctx, name, dockertypes.ContainerRemoveOptions{
		Force: true,
	})
}