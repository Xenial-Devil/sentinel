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
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

// Label keys for per-container registry credential override
const (
	LabelRegistryUser = "com.sentinel.registry.user"
	LabelRegistryPass = "com.sentinel.registry.pass"
)

// UpdateResult holds the result of an update
type UpdateResult struct {
	ContainerName string
	Image         string
	OldImage      string
	NewImage      string
	Updated       bool
	RolledBack    bool
	NoPull        bool
	NoRestart     bool
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
func (u *Updater) CheckAndUpdate(
	ct docker.ContainerInfo,
	noPull bool,
	noRestart bool,
	rollingRestart bool,
) UpdateResult {
	result := UpdateResult{
		ContainerName: ct.Name,
		Image:         ct.Image,
		OldImage:      ct.Image,
		NoPull:        noPull,
		NoRestart:     noRestart,
	}

	// ── Extract per-container label credentials ───────────────────────────────
	labelUser := ct.Labels[LabelRegistryUser]
	labelPass := ct.Labels[LabelRegistryPass]

	if labelUser != "" && labelPass != "" {
		logger.Log.Debugf("Container %s has label credentials — using override for %s",
			ct.Name, ct.Image)
	}

	logger.LogImageCheckStart(ct.Name, ct.Image)

	// ── No-pull mode ──────────────────────────────────────────────────────────
	if noPull {
		logger.Log.Infof("👁   [NO-PULL] %s - skipping registry check", ct.Name)

		if noRestart {
			logger.Log.Infof("👁   [NO-RESTART] %s - no action taken", ct.Name)
			return result
		}

		if rollingRestart || u.Config.RollingRestart {
			return u.applyRollingRestart(ct, result, labelUser, labelPass)
		}

		return u.applyRestartOnly(ct, result)
	}

	// ── Get local digest ──────────────────────────────────────────────────────
	localDigest, err := u.GetLocalDigest(ct.ImageID)
	if err != nil {
		logger.Log.Errorf("Failed to get local digest for %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	// ── Check registry (with label creds if present) ──────────────────────────
	hasUpdate, _, err := u.Registry.HasUpdateWithCreds(localDigest, ct.Image, labelUser, labelPass)
	if err != nil {
		logger.Log.Warnf("Failed to check registry for %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	if !hasUpdate {
		currentTag := GetTagFromImage(ct.Image)
		logger.LogImageUpToDate(ct.Name, ct.Image, currentTag)
		return result
	}

	// ── Semver policy check ───────────────────────────────────────────────────
	currentTag := GetTagFromImage(ct.Image)
	newTag := currentTag

	policy := Policy(u.Config.SemverPolicy)
	if policy != PolicyAll {
		allowed, err := CheckVersionPolicy(currentTag, newTag, policy)
		if err != nil {
			logger.Log.Warnf("Version policy check failed for %s: %v", ct.Name, err)
		}
		if !allowed {
			logger.Log.Infof("Update blocked by semver policy (%s) for %s", policy, ct.Name)
			return result
		}
	}

	logger.LogImageUpdateFound(ct.Name, ct.Image, currentTag, newTag)

	// ── No-restart: pull only ─────────────────────────────────────────────────
	if noRestart || u.Config.NoRestart {
		logger.Log.Infof("📥  [NO-RESTART] %s pulling image but not restarting", ct.Name)
		if err := u.pullImage(ct.Image, labelUser, labelPass); err != nil {
			logger.LogImagePullFailed(ct.Name, ct.Image, err)
			result.Error = err
			return result
		}
		logger.Log.Infof("📦  [NO-RESTART] %s image pulled - container not restarted", ct.Name)
		result.Updated = true
		result.NewImage = ct.Image
		return result
	}

	// ── Rolling restart ───────────────────────────────────────────────────────
	if rollingRestart || u.Config.RollingRestart {
		return u.applyRollingRestart(ct, result, labelUser, labelPass)
	}

	// ── Standard update ───────────────────────────────────────────────────────
	start := time.Now()
	if err := u.applyUpdate(ct, &result, labelUser, labelPass); err != nil {
		logger.Log.Errorf("Failed to update %s: %v", ct.Name, err)
		result.Error = err
		return result
	}

	result.Updated = true
	result.NewImage = ct.Image
	logger.LogUpdateSuccess(ct.Name, ct.Image, ct.Image, time.Since(start))

	return result
}

// applyUpdate pulls new image, stops old container, recreates with health check
func (u *Updater) applyUpdate(
	ct docker.ContainerInfo,
	result *UpdateResult,
	labelUser, labelPass string,
) error {
	info, err := u.Client.InspectContainer(ct.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}

	rollbackState := u.SaveState(info)

	// Pull
	logger.LogImagePullStart(ct.Name, ct.Image)
	pullStart := time.Now()
	if err := u.pullImage(ct.Image, labelUser, labelPass); err != nil {
		logger.LogImagePullFailed(ct.Name, ct.Image, err)
		return fmt.Errorf("failed to pull image: %v", err)
	}
	logger.LogImagePullSuccess(ct.Name, ct.Image, time.Since(pullStart))

	// Stop
	stopStart := time.Now()
	logger.LogContainerStopping(ct.Name, ct.ID, u.Config.StopTimeout)
	if err := u.Client.StopContainer(ct.ID, u.Config.StopTimeout); err != nil {
		return fmt.Errorf("failed to stop container: %v", err)
	}
	logger.LogContainerStopped(ct.Name, ct.ID, time.Since(stopStart))

	// Remove
	logger.LogContainerRemoving(ct.Name, ct.ID)
	if err := u.Client.RemoveContainer(ct.ID); err != nil {
		return fmt.Errorf("failed to remove container: %v", err)
	}
	logger.LogContainerRemoved(ct.Name, ct.ID)

	// Recreate
	logger.LogContainerStarting(ct.Name, ct.ID)
	startTime := time.Now()
	if err := u.recreateContainer(ct.Name, info); err != nil {
		logger.Log.Errorf("Recreate failed for %s - attempting rollback", ct.Name)
		if u.Config.EnableRollback {
			if rbErr := u.Rollback(rollbackState); rbErr != nil {
				logger.LogRollbackFailed(ct.Name, ct.Image, rbErr)
				return fmt.Errorf("recreate and rollback both failed: %v / %v", err, rbErr)
			}
			result.RolledBack = true
			result.NewImage = rollbackState.OldImage
			logger.LogRollbackSuccess(ct.Name, rollbackState.OldImage)
		}
		return fmt.Errorf("failed to recreate container: %v", err)
	}
	logger.LogContainerStarted(ct.Name, ct.ID, time.Since(startTime))

	// Health check + rollback
	if u.Config.EnableRollback {
		if err := u.WaitForHealthy(ct.Name); err != nil {
			logger.LogHealthTimeout(ct.Name, u.Config.HealthTimeout)
			logger.LogRollbackStart(ct.Name, ct.Image, rollbackState.OldImage)

			if rbErr := u.Rollback(rollbackState); rbErr != nil {
				logger.LogRollbackFailed(ct.Name, ct.Image, rbErr)
				return fmt.Errorf("health failed and rollback failed: %v / %v", err, rbErr)
			}

			result.RolledBack = true
			result.NewImage = rollbackState.OldImage
			logger.LogRollbackSuccess(ct.Name, rollbackState.OldImage)
			return fmt.Errorf("container unhealthy after update - rolled back")
		}
		logger.LogHealthCheck(ct.Name, true, time.Duration(u.Config.HealthTimeout)*time.Second)
	}

	return nil
}

// applyRollingRestart stops and restarts container with optional image pull
func (u *Updater) applyRollingRestart(
	ct docker.ContainerInfo,
	result UpdateResult,
	labelUser, labelPass string,
) UpdateResult {
	logger.Log.Infof("🔄  [ROLLING-RESTART] Starting rolling restart for %s", ct.Name)

	info, err := u.Client.InspectContainer(ct.ID)
	if err != nil {
		result.Error = fmt.Errorf("failed to inspect container: %v", err)
		return result
	}

	rollbackState := u.SaveState(info)

	if !result.NoPull {
		logger.LogImagePullStart(ct.Name, ct.Image)
		pullStart := time.Now()
		if err := u.pullImage(ct.Image, labelUser, labelPass); err != nil {
			logger.LogImagePullFailed(ct.Name, ct.Image, err)
			result.Error = err
			return result
		}
		logger.LogImagePullSuccess(ct.Name, ct.Image, time.Since(pullStart))
	}

	logger.LogContainerStopping(ct.Name, ct.ID, u.Config.StopTimeout)
	if err := u.Client.StopContainer(ct.ID, u.Config.StopTimeout); err != nil {
		result.Error = fmt.Errorf("rolling restart stop failed: %v", err)
		return result
	}

	time.Sleep(2 * time.Second)

	if err := u.Client.RemoveContainer(ct.ID); err != nil {
		result.Error = fmt.Errorf("rolling restart remove failed: %v", err)
		return result
	}

	if err := u.recreateContainer(ct.Name, info); err != nil {
		if u.Config.EnableRollback {
			if rbErr := u.Rollback(rollbackState); rbErr != nil {
				logger.LogRollbackFailed(ct.Name, ct.Image, rbErr)
			} else {
				result.RolledBack = true
				result.NewImage = rollbackState.OldImage
				logger.LogRollbackSuccess(ct.Name, rollbackState.OldImage)
			}
		}
		result.Error = fmt.Errorf("rolling restart recreate failed: %v", err)
		return result
	}

	if u.Config.EnableRollback {
		if err := u.WaitForHealthy(ct.Name); err != nil {
			logger.LogRollbackStart(ct.Name, ct.Image, rollbackState.OldImage)
			if rbErr := u.Rollback(rollbackState); rbErr != nil {
				logger.LogRollbackFailed(ct.Name, ct.Image, rbErr)
			} else {
				result.RolledBack = true
				result.NewImage = rollbackState.OldImage
				logger.LogRollbackSuccess(ct.Name, rollbackState.OldImage)
			}
			result.Error = err
			return result
		}
	}

	logger.Log.Infof("✅  [ROLLING-RESTART] %s restarted successfully", ct.Name)
	result.Updated = true
	result.NewImage = ct.Image
	return result
}

// applyRestartOnly restarts container without pulling new image
func (u *Updater) applyRestartOnly(ct docker.ContainerInfo, result UpdateResult) UpdateResult {
	logger.Log.Infof("🔄  [RESTART-ONLY] Restarting %s without pull", ct.Name)

	ctx := context.Background()
	timeout := u.Config.StopTimeout

	if err := u.Client.CLI.ContainerRestart(ctx, ct.ID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil {
		result.Error = fmt.Errorf("restart failed: %v", err)
		return result
	}

	logger.Log.Infof("✅  [RESTART-ONLY] %s restarted", ct.Name)
	result.Updated = true
	return result
}

// pullImage pulls a new image from registry.
// Uses label credentials if provided, otherwise falls back to global credentials.
func (u *Updater) pullImage(image, labelUser, labelPass string) error {
	ctx := context.Background()

	ref := registry.ParseImageRef(image)

	// Resolve auth header: label creds > global creds
	authHeader := registry.GetAuthHeaderWithCreds(ref.Registry, labelUser, labelPass)

	reader, err := u.Client.CLI.ImagePull(
		ctx,
		image,
		dockertypes.ImagePullOptions{
			RegistryAuth: authHeader,
		},
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

	return nil
}

// recreateContainer creates a new container with saved settings
func (u *Updater) recreateContainer(name string, info dockertypes.ContainerJSON) error {
	ctx := context.Background()

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: info.NetworkSettings.Networks,
	}

	resp, err := u.Client.CLI.ContainerCreate(
		ctx,
		info.Config,
		info.HostConfig,
		networkConfig,
		nil,
		name,
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	if err := u.Client.CLI.ContainerStart(
		ctx,
		resp.ID,
		dockertypes.ContainerStartOptions{},
	); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	return nil
}

// GetLocalDigest gets the digest of a local image
func (u *Updater) GetLocalDigest(imageID string) (string, error) {
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
		dockertypes.ImageRemoveOptions{Force: false},
	)
	if err != nil {
		logger.Log.Warnf("Failed to cleanup old image %s: %v", imageID, err)
		return
	}

	logger.LogCleanup(imageID, 0)
}