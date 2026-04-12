package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
	"sentinel/registry"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
)

// UpdateResult holds the result of an update
type UpdateResult struct {
	ContainerName string
	Image         string
	Updated       bool
	RolledBack    bool
	Error         error
}

// Updater handles container updates
type Updater struct {
	Client   *docker.Client
	Config   *config.Config
	Registry *registry.Client
}

// New creates a new Updater
func New(client *docker.Client, cfg *config.Config) *Updater {
	return &Updater{
		Client:   client,
		Config:   cfg,
		Registry: registry.New(),
	}
}

// CheckAndUpdate checks a container for updates and applies if available
func (u *Updater) CheckAndUpdate(ct docker.ContainerInfo) UpdateResult {
	result := UpdateResult{
		ContainerName: ct.Name,
		Image:         ct.Image,
	}

	localDigest, err := u.getLocalDigest(ct.ImageID)
	if err != nil {
		logger.Log.Errorf("Failed to get local digest for %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	hasUpdate, newTag, err := u.Registry.HasUpdate(localDigest, ct.Image)
	if err != nil {
		logger.Log.Warnf("Failed to check registry for %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	if !hasUpdate {
		logger.Log.Infof("No update for %s", ct.Name)
		return result
	}

	currentTag := GetTagFromImage(ct.Image)
	policy := Policy(u.Config.SemverPolicy)
	allowed, err := CheckVersionPolicy(currentTag, newTag, policy)
	if err != nil {
		logger.Log.Warnf("Version policy check failed for %s: %v", ct.Name, err)
	}
	if !allowed {
		logger.Log.Infof("Update blocked by policy (%s) for %s: %s -> %s",
			policy, ct.Name, currentTag, newTag)
		return result
	}

	logger.Log.Infof("Update found for %s (%s -> %s) - applying...", ct.Name, currentTag, newTag)
	err = u.applyUpdate(ct)
	if err != nil {
		logger.Log.Errorf("Failed to update %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	result.Updated = true
	logger.Log.Infof("Successfully updated %s", ct.Name)
	return result
}

// applyUpdate pulls new image and recreates container
func (u *Updater) applyUpdate(ct docker.ContainerInfo) error {
	info, err := u.Client.InspectContainer(ct.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}

	logger.Log.Infof("Pulling new image: %s", ct.Image)
	err = u.pullImage(ct.Image)
	if err != nil {
		return fmt.Errorf("failed to pull image: %v", err)
	}

	logger.Log.Infof("Stopping container: %s", ct.Name)
	err = u.Client.StopContainer(ct.ID, u.Config.StopTimeout)
	if err != nil {
		return fmt.Errorf("failed to stop container: %v", err)
	}

	logger.Log.Infof("Removing old container: %s", ct.Name)
	err = u.Client.RemoveContainer(ct.ID)
	if err != nil {
		return fmt.Errorf("failed to remove container: %v", err)
	}

	logger.Log.Infof("Recreating container: %s", ct.Name)
	err = u.recreateContainer(ct.Name, info)
	if err != nil {
		return fmt.Errorf("failed to recreate container: %v", err)
	}

	return nil
}

// pullImage pulls a new image from registry
func (u *Updater) pullImage(image string) error {
	ctx := context.Background()

	reader, err := u.Client.CLI.ImagePull(
		ctx,
		image,
		dockertypes.ImagePullOptions{},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Log.Warnf("Failed to close image pull reader: %v", err)
		}
	}()

	decoder := json.NewDecoder(reader)
	for {
		var event map[string]interface{}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if status, ok := event["status"].(string); ok {
			if id, ok := event["id"].(string); ok {
				logger.Log.Debugf("Pull: %s - %s", id, status)
			}
		}
	}

	logger.Log.Infof("Image pulled successfully: %s", image)
	return nil
}

// recreateContainer creates a new container with saved settings
func (u *Updater) recreateContainer(name string, info dockertypes.ContainerJSON) error {
	ctx := context.Background()

	containerConfig := info.Config
	hostConfig := info.HostConfig

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: info.NetworkSettings.Networks,
	}

	resp, err := u.Client.CLI.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		name,
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	err = u.Client.CLI.ContainerStart(
		ctx,
		resp.ID,
		dockertypes.ContainerStartOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	logger.Log.Infof("Container recreated and started: %s", name)
	return nil
}

// getLocalDigest gets the digest of a local image
func (u *Updater) getLocalDigest(imageID string) (string, error) {
	ctx := context.Background()

	inspect, _, err := u.Client.CLI.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return "", err
	}

	if len(inspect.RepoDigests) > 0 {
		parts := strings.SplitN(inspect.RepoDigests[0], "@", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
		return inspect.RepoDigests[0], nil
	}

	return inspect.ID, nil
}

// CleanupOldImage removes old image after successful update
func (u *Updater) CleanupOldImage(imageID string) {
	if !u.Config.Cleanup {
		return
	}

	ctx := context.Background()

	_, err := u.Client.CLI.ImageRemove(
		ctx,
		imageID,
		dockertypes.ImageRemoveOptions{
			Force: false,
		},
	)
	if err != nil {
		logger.Log.Warnf("Failed to cleanup old image %s: %v", imageID, err)
		return
	}

	logger.Log.Infof("Old image removed: %s", imageID)
}