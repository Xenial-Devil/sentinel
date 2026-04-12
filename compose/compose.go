package compose

import (
	"context"
	"sentinel/logger"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const (
	// Docker compose labels
	LabelProject = "com.docker.compose.project"
	LabelService = "com.docker.compose.service"
	LabelNumber  = "com.docker.compose.container-number"
)

// Project represents a docker compose project
type Project struct {
	Name       string
	Services   map[string]*Service
}

// Service represents a service in a compose project
type Service struct {
	Name        string
	ProjectName string
	Containers  []Container
}

// Container represents a container in a service
type Container struct {
	ID     string
	Name   string
	Image  string
	Status string
	Labels map[string]string
}

// Detector finds compose projects from running containers
type Detector struct {
	Client *client.Client
}

// New creates a new compose Detector
func New(cli *client.Client) *Detector {
	return &Detector{
		Client: cli,
	}
}

// DetectProjects finds all running compose projects
func (d *Detector) DetectProjects() (map[string]*Project, error) {
	ctx := context.Background()

	// Filter containers with compose label
	f := filters.NewArgs()
	f.Add("label", LabelProject)

	containers, err := d.Client.ContainerList(ctx, types.ContainerListOptions{
		All:     false,
		Filters: f,
	})
	if err != nil {
		logger.Log.Errorf("Failed to list compose containers: %v", err)
		return nil, err
	}

	// Group containers by project
	projects := make(map[string]*Project)

	for _, ct := range containers {
		// Get project name
		projectName := ct.Labels[LabelProject]
		if projectName == "" {
			continue
		}

		// Get service name
		serviceName := ct.Labels[LabelService]
		if serviceName == "" {
			continue
		}

		// Create project if not exists
		if _, ok := projects[projectName]; !ok {
			projects[projectName] = &Project{
				Name:     projectName,
				Services: make(map[string]*Service),
			}
		}

		// Create service if not exists
		if _, ok := projects[projectName].Services[serviceName]; !ok {
			projects[projectName].Services[serviceName] = &Service{
				Name:        serviceName,
				ProjectName: projectName,
				Containers:  []Container{},
			}
		}

		// Get container name
		name := "unknown"
		if len(ct.Names) > 0 {
			name = ct.Names[0][1:]
		}

		// Add container to service
		projects[projectName].Services[serviceName].Containers = append(
			projects[projectName].Services[serviceName].Containers,
			Container{
				ID:     ct.ID[:12],
				Name:   name,
				Image:  ct.Image,
				Status: ct.Status,
				Labels: ct.Labels,
			},
		)
	}

	logger.Log.Infof("Detected %d compose projects", len(projects))
	return projects, nil
}

// GetProjectForContainer returns project name for a container
func GetProjectForContainer(labels map[string]string) string {
	return labels[LabelProject]
}

// GetServiceForContainer returns service name for a container
func GetServiceForContainer(labels map[string]string) string {
	return labels[LabelService]
}

// IsComposeContainer checks if container is part of compose project
func IsComposeContainer(labels map[string]string) bool {
	_, ok := labels[LabelProject]
	return ok
}