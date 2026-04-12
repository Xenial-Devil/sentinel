package health

import (
	"context"
	"fmt"
	"sentinel/logger"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// Status represents container health status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusStarting  Status = "starting"
	StatusNone      Status = "none"
	StatusUnknown   Status = "unknown"
)

// Result holds health check result
type Result struct {
	ContainerName string
	Status        Status
	Message       string
	CheckedAt     time.Time
}

// Checker performs health checks on containers
type Checker struct {
	Client  *client.Client
	Timeout int
}

// New creates a new health Checker
func New(cli *client.Client, timeout int) *Checker {
	return &Checker{
		Client:  cli,
		Timeout: timeout,
	}
}

// Check performs a health check on a container
func (c *Checker) Check(containerName string) Result {
	result := Result{
		ContainerName: containerName,
		CheckedAt:     time.Now(),
	}

	// Inspect container
	info, err := c.inspectContainer(containerName)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("Failed to inspect: %v", err)
		return result
	}

	// Check container state
	if !info.State.Running {
		result.Status = StatusUnhealthy
		result.Message = "Container is not running"
		return result
	}

	// Check health status
	if info.State.Health == nil {
		result.Status = StatusNone
		result.Message = "No healthcheck configured"
		return result
	}

	// Map health status
	switch info.State.Health.Status {
	case "healthy":
		result.Status = StatusHealthy
		result.Message = "Container is healthy"

	case "unhealthy":
		result.Status = StatusUnhealthy
		result.Message = getLastHealthLog(info)

	case "starting":
		result.Status = StatusStarting
		result.Message = "Container is starting"

	default:
		result.Status = StatusUnknown
		result.Message = "Unknown health status"
	}

	return result
}

// WaitUntilHealthy waits for container to become healthy
func (c *Checker) WaitUntilHealthy(containerName string) error {
	logger.Log.Infof("Waiting for %s to become healthy...", containerName)

	deadline := time.Now().Add(time.Duration(c.Timeout) * time.Second)

	for time.Now().Before(deadline) {
		result := c.Check(containerName)

		switch result.Status {
		case StatusHealthy:
			logger.Log.Infof("Container %s is healthy", containerName)
			return nil

		case StatusNone:
			// No healthcheck - wait a bit and assume healthy
			logger.Log.Debugf("No healthcheck for %s - waiting 5s then assuming healthy",
				containerName)
			time.Sleep(5 * time.Second)
			return nil

		case StatusUnhealthy:
			return fmt.Errorf("container %s is unhealthy: %s",
				containerName,
				result.Message,
			)

		case StatusStarting:
			logger.Log.Debugf("Container %s is starting - waiting...", containerName)
			time.Sleep(2 * time.Second)

		default:
			logger.Log.Debugf("Container %s status unknown - waiting...", containerName)
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("health timeout after %d seconds for %s",
		c.Timeout,
		containerName,
	)
}

// IsRunning checks if a container is running
func (c *Checker) IsRunning(containerName string) (bool, error) {
	info, err := c.inspectContainer(containerName)
	if err != nil {
		return false, err
	}
	return info.State.Running, nil
}

// inspectContainer inspects a container
func (c *Checker) inspectContainer(containerName string) (dockertypes.ContainerJSON, error) {
	ctx := context.Background()

	info, err := c.Client.ContainerInspect(ctx, containerName)
	if err != nil {
		return dockertypes.ContainerJSON{}, err
	}

	return info, nil
}

// getLastHealthLog returns last health check log message
func getLastHealthLog(info dockertypes.ContainerJSON) string {
	if info.State.Health == nil {
		return "no health info"
	}

	logs := info.State.Health.Log
	if len(logs) == 0 {
		return "no health logs"
	}

	// Return last log output
	last := logs[len(logs)-1]
	if last.Output != "" {
		return last.Output
	}

	return fmt.Sprintf("exit code: %d", last.ExitCode)
}