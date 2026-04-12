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
		ContainerName: ct.Name[1:], // remove leading /
		OldImageID:    ct.Image,
		OldImage:      ct.Config.Image,
		Info:          ct,
	}
}

// Rollback reverts a container to its previous state
func (u *Updater) Rollback(state *RollbackState) error {
	logger.Log.Warnf("Rolling back container: %s", state.ContainerName)

	// Step 1: Stop current (broken) container
	err := u.stopCurrentContainer(state.ContainerName)
	if err != nil {
		logger.Log.Errorf("Failed to stop broken container: %v", err)
	}

	// Step 2: Remove current (broken) container
	err = u.removeCurrentContainer(state.ContainerName)
	if err != nil {
		logger.Log.Errorf("Failed to remove broken container: %v", err)
	}

	// Step 3: Recreate with old image
	logger.Log.Infof("Restoring old image: %s", state.OldImage)

	// Set old image in config
	state.Info.Config.Image = state.OldImage

	// Recreate with old settings
	err = u.recreateContainer(state.ContainerName, state.Info)
	if err != nil {
		return fmt.Errorf("rollback failed: %v", err)
	}

	logger.Log.Infof("Rollback successful for: %s", state.ContainerName)
	return nil
}

// WaitForHealthy waits for container to become healthy
func (u *Updater) WaitForHealthy(containerName string) error {
	logger.Log.Infof("Waiting for %s to become healthy...", containerName)

	timeout := time.Duration(u.Config.HealthTimeout) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check container health
		status, err := u.getHealthStatus(containerName)
		if err != nil {
			logger.Log.Warnf("Failed to get health status: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		switch status {
		case "healthy":
			logger.Log.Infof("Container %s is healthy", containerName)
			return nil

		case "unhealthy":
			return fmt.Errorf("container %s is unhealthy", containerName)

		case "none":
			// No healthcheck defined - assume healthy
			logger.Log.Debugf("No healthcheck for %s - assuming healthy", containerName)
			return nil

		default:
			// Still starting
			logger.Log.Debugf("Container %s status: %s - waiting...", containerName, status)
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("container %s health timeout after %d seconds",
		containerName,
		u.Config.HealthTimeout,
	)
}

// CheckAndRollback updates container and rolls back if unhealthy
func (u *Updater) CheckAndRollback(ct dockertypes.ContainerJSON) error {
	// Save state before update
	state := u.SaveState(ct)

	containerName := ct.Name[1:]

	// Wait for healthy state
	err := u.WaitForHealthy(containerName)
	if err != nil {
		logger.Log.Warnf("Container unhealthy after update: %v", err)

		// Rollback if enabled
		if u.Config.EnableRollback {
			rollbackErr := u.Rollback(state)
			if rollbackErr != nil {
				return fmt.Errorf("update failed and rollback failed: %v", rollbackErr)
			}
			return fmt.Errorf("update failed - rolled back to previous version")
		}

		return err
	}

	return nil
}

// getHealthStatus returns health status of a container
func (u *Updater) getHealthStatus(containerName string) (string, error) {
	ctx := context.Background()

	info, err := u.Client.CLI.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}

	// No health check configured
	if info.State.Health == nil {
		return "none", nil
	}

	return info.State.Health.Status, nil
}

// stopCurrentContainer stops a container by name
func (u *Updater) stopCurrentContainer(name string) error {
	ctx := context.Background()

	timeout := u.Config.StopTimeout
	err := u.Client.CLI.ContainerStop(ctx, name, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		return err
	}

	logger.Log.Debugf("Stopped container: %s", name)
	return nil
}

// removeCurrentContainer removes a container by name
func (u *Updater) removeCurrentContainer(name string) error {
	ctx := context.Background()

	err := u.Client.CLI.ContainerRemove(ctx, name, dockertypes.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		return err
	}

	logger.Log.Debugf("Removed container: %s", name)
	return nil
}