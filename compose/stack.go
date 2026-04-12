package compose

import (
	"context"
	"fmt"
	"sentinel/logger"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// StackUpdater handles updating compose stacks
type StackUpdater struct {
	Client  *client.Client
	Timeout int
}

// NewStackUpdater creates a new StackUpdater
func NewStackUpdater(cli *client.Client, timeout int) *StackUpdater {
	return &StackUpdater{
		Client:  cli,
		Timeout: timeout,
	}
}

// UpdateResult holds result of stack update
type UpdateResult struct {
	ProjectName string
	ServiceName string
	Success     bool
	Error       error
}

// UpdateProject updates all services in a project
func (s *StackUpdater) UpdateProject(project *Project) []UpdateResult {
	logger.Log.Infof("Updating compose project: %s", project.Name)

	var results []UpdateResult

	// Update each service
	for _, service := range project.Services {
		result := s.UpdateService(service)
		results = append(results, result)

		// Stop if service failed
		if !result.Success {
			logger.Log.Errorf("Service %s failed - stopping project update",
				service.Name)
			break
		}

		// Small delay between services
		time.Sleep(1 * time.Second)
	}

	return results
}

// UpdateService updates all containers in a service
func (s *StackUpdater) UpdateService(service *Service) UpdateResult {
	logger.Log.Infof("Updating service: %s in project: %s",
		service.Name,
		service.ProjectName,
	)

	result := UpdateResult{
		ProjectName: service.ProjectName,
		ServiceName: service.Name,
	}

	// Update each container in service
	for _, ct := range service.Containers {
		err := s.restartContainer(ct.ID)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("failed to restart %s: %v", ct.Name, err)
			logger.Log.Errorf("Failed to restart container %s: %v", ct.Name, err)
			return result
		}

		logger.Log.Infof("Restarted container: %s", ct.Name)
	}

	result.Success = true
	logger.Log.Infof("Service updated: %s", service.Name)
	return result
}

// GetProjectSummary returns a summary of a project
func GetProjectSummary(project *Project) map[string]interface{} {
	services := make(map[string]interface{})

	for name, service := range project.Services {
		containers := make([]map[string]string, 0)

		for _, ct := range service.Containers {
			containers = append(containers, map[string]string{
				"id":     ct.ID,
				"name":   ct.Name,
				"image":  ct.Image,
				"status": ct.Status,
			})
		}

		services[name] = map[string]interface{}{
			"name":       service.Name,
			"containers": containers,
		}
	}

	return map[string]interface{}{
		"project":  project.Name,
		"services": services,
	}
}

// restartContainer restarts a container by ID
func (s *StackUpdater) restartContainer(id string) error {
	ctx := context.Background()

	timeout := s.Timeout
	err := s.Client.ContainerRestart(ctx, id, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		return err
	}

	return nil
}

// LogProjectStatus logs the status of all services in a project
func LogProjectStatus(project *Project) {
	logger.Log.Infof("Project: %s", project.Name)

	for _, service := range project.Services {
		logger.Log.Infof("  Service: %s", service.Name)

		for _, ct := range service.Containers {
			logger.Log.Infof("    Container: %s | Image: %s | Status: %s",
				ct.Name,
				ct.Image,
				ct.Status,
			)
		}
	}
}