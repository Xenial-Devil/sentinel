package watcher

import (
	"fmt"
	"sentinel/approval"
	"sentinel/config"
	"sentinel/docker"
	"sentinel/hooks"
	"sentinel/logger"
	"sentinel/metrics"
	"sentinel/notifier"
	"sentinel/scheduler"
	"sentinel/updater"
	"sentinel/webhook"
	"sync"
	"time"
)

// Watcher monitors Docker containers
type Watcher struct {
	Client    *docker.Client
	Config    *config.Config
	Updater   *updater.Updater
	Scheduler *scheduler.Scheduler
	Notifier  *notifier.Notifier
	Webhook   *webhook.Client
	Approval  *approval.Manager
	Metrics   *metrics.Metrics
	Hooks     *hooks.Runner
	cycleNum  int
	mu        sync.Mutex
}

// New creates a new Watcher with all dependencies wired
func New(client *docker.Client, cfg *config.Config, m *metrics.Metrics) *Watcher {
	w := &Watcher{
		Client:  client,
		Config:  cfg,
		Updater: updater.New(client, cfg),
		Metrics: m,
	}

	w.Notifier = notifier.New(cfg)
	w.Hooks = buildHookRunner(cfg)

	if cfg.WebhookURL != "" {
		w.Webhook = webhook.New(cfg.WebhookURL, cfg.WebhookSecret)
		logger.Log.Info("🔔  Webhook notifications enabled")
	}

	if cfg.ApprovalEnabled {
		w.Approval = approval.GetInstance(cfg.ApprovalFilePath)
		logger.Log.Info("✋  Approval mode enabled")
	}

	w.Scheduler = scheduler.New(cfg, w.RunCycle)
	return w
}

// buildHookRunner builds the global hook runner from config
func buildHookRunner(cfg *config.Config) *hooks.Runner {
	r := hooks.New()

	type hookDef struct {
		hookType hooks.HookType
		cmd      string
	}

	defs := []hookDef{
		{hooks.HookPreCheck, cfg.HookPreCheck},
		{hooks.HookPostCheck, cfg.HookPostCheck},
		{hooks.HookPreUpdate, cfg.HookPreUpdate},
		{hooks.HookPostUpdate, cfg.HookPostUpdate},
		{hooks.HookPreRollback, cfg.HookPreRollback},
		{hooks.HookPostRollback, cfg.HookPostRollback},
	}

	for _, d := range defs {
		if d.cmd != "" {
			r.Register(hooks.Hook{
				Type:    d.hookType,
				Command: d.cmd,
				Timeout: cfg.HookTimeout,
			})
		}
	}

	return r
}

// Start begins the watch loop
func (w *Watcher) Start() {
	logger.Log.Info("🛡   Sentinel is starting...")
	w.Notifier.NotifyStartup()

	if w.Config.RunOnce {
		logger.Log.Info("🔂  Run-once mode: executing single cycle then exiting")
		w.RunCycle()
		logger.Log.Info("🔂  Run-once complete - exiting")
		return
	}

	w.Scheduler.Start()
}

// RunCycle runs one full check cycle
func (w *Watcher) RunCycle() {
	w.mu.Lock()
	w.cycleNum++
	cycleNum := w.cycleNum
	w.mu.Unlock()

	start := time.Now()
	logger.LogCycleStart(cycleNum)

	containers, err := w.Client.ListContainersWithOptions(
		w.Config.IncludeStopped,
		w.Config.IncludeRestarting,
	)
	if err != nil {
		logger.Log.Errorf("Failed to list containers: %v", err)
		return
	}

	if w.Config.ReviveStopped {
		w.reviveStopped(containers)
	}

	filtered := Filter(containers, w.Config)

	if w.Metrics != nil {
		w.Metrics.SetContainersWatched(len(filtered))
	}

	var updated, skipped, failed int

	for _, ct := range filtered {
		switch w.CheckContainer(ct) {
		case "updated":
			updated++
		case "failed":
			failed++
		default:
			skipped++
		}
	}

	elapsed := time.Since(start)
	logger.LogCycleEnd(cycleNum, len(filtered), updated, skipped, failed, elapsed)

	if w.Approval != nil && w.Metrics != nil {
		w.Metrics.SetUpdatesPending(len(w.Approval.GetPending()))
	}
}

// reviveStopped starts watched stopped containers
func (w *Watcher) reviveStopped(containers []docker.ContainerInfo) {
	for _, ct := range containers {
		if ct.State == "exited" || ct.State == "created" {
			filtered := Filter([]docker.ContainerInfo{ct}, w.Config)
			if len(filtered) == 0 {
				continue
			}
			logger.Log.Infof("♻️   Reviving stopped container: %s", ct.Name)
			if err := w.Client.ReviveContainer(ct.ID, ct.Name); err != nil {
				logger.Log.Errorf("Failed to revive %s: %v", ct.Name, err)
			}
		}
	}
}

// CheckContainer checks a single container
func (w *Watcher) CheckContainer(ct docker.ContainerInfo) string {
	if ct.Name == "sentinel" {
		return "skipped"
	}

	logger.LogContainerFound(ct.Name, ct.Image, ct.Status)

	// Load per-container hooks from labels and merge with global hooks
	containerHooks := hooks.ParseHooksFromLabels(ct.Labels)
	for _, h := range containerHooks {
		w.Hooks.Register(h)
	}

	// pre-check hook
	if err := w.Hooks.Run(hooks.HookPreCheck, ct.Name, ct.Image); err != nil {
		logger.Log.Warnf("Pre-check hook failed for %s: %v - skipping", ct.Name, err)
		return "skipped"
	}

	if w.Config.MonitorOnly || IsMonitorOnly(ct) {
		logger.LogContainerMonitorOnly(ct.Name, ct.Image, ct.Image, "", false)
		w.Hooks.RunSoft(hooks.HookPostCheck, ct.Name, ct.Image)
		return "skipped"
	}

	result := w.runUpdate(ct)

	// post-check hook always runs
	w.Hooks.RunSoft(hooks.HookPostCheck, ct.Name, ct.Image)

	return result
}

// runUpdate runs the full update pipeline
func (w *Watcher) runUpdate(ct docker.ContainerInfo) string {
	logger.LogContainerWatched(ct.Name, ct.Image)

	noPull         := w.Config.NoPull || IsNoPull(ct)
	noRestart      := w.Config.NoRestart || IsNoRestart(ct)
	rollingRestart := w.Config.RollingRestart || IsRollingRestart(ct)

	if w.Config.ApprovalEnabled && w.Approval != nil && !noPull {
		return w.runWithApproval(ct, noRestart, rollingRestart)
	}

	return w.executeUpdate(ct, noPull, noRestart, rollingRestart)
}

// runWithApproval gates update on approval after detecting real update
func (w *Watcher) runWithApproval(
	ct docker.ContainerInfo,
	noRestart bool,
	rollingRestart bool,
) string {
	localDigest, err := w.Updater.GetLocalDigest(ct.ImageID)
	if err != nil {
		logger.Log.Warnf("Could not get local digest for %s: %v", ct.Name, err)
		return "skipped"
	}

	hasUpdate, remoteDigest, err := w.Updater.Registry.HasUpdate(localDigest, ct.Image)
	if err != nil {
		logger.Log.Warnf("Registry check failed for %s: %v", ct.Name, err)
		return "skipped"
	}

	if !hasUpdate {
		logger.LogImageUpToDate(ct.Name, ct.Image, updater.GetTagFromImage(ct.Image))
		return "skipped"
	}

	newImageID := ct.Image + "@" + remoteDigest

	if w.Approval.IsApproved(ct.Name, newImageID) {
		logger.LogApprovalGranted(ct.Name, newImageID)
		return w.executeUpdate(ct, false, noRestart, rollingRestart)
	}

	if w.Approval.IsPending(ct.Name, newImageID) {
		logger.Log.Infof("⏳  Waiting for approval: %s", ct.Name)
		return "skipped"
	}

	w.Approval.RequestApproval(ct.Name, ct.Image, newImageID)
	logger.LogApprovalRequired(ct.Name, ct.Image, ct.Image, newImageID)
	w.sendWebhook(webhook.EventApprovalPending, ct.Name, ct.Image, "", nil)

	return "skipped"
}

// executeUpdate runs the actual update with hooks and result handling
func (w *Watcher) executeUpdate(
	ct docker.ContainerInfo,
	noPull bool,
	noRestart bool,
	rollingRestart bool,
) string {
	// pre-update hook - failure blocks update
	if err := w.Hooks.Run(hooks.HookPreUpdate, ct.Name, ct.Image); err != nil {
		logger.Log.Errorf("Pre-update hook failed for %s: %v - aborting update", ct.Name, err)
		return "failed"
	}

	if !noPull {
		w.sendWebhook(webhook.EventPullStarted, ct.Name, ct.Image, "", nil)
	}

	result := w.Updater.CheckAndUpdate(ct, noPull, noRestart, rollingRestart)

	if result.Error != nil {
		logger.LogUpdateFailed(ct.Name, ct.Image, result.Error)

		if w.Metrics != nil {
			w.Metrics.RecordUpdateFailed()
		}

		w.Notifier.NotifyFailed(ct.Name, ct.Image, result.Error)
		w.sendWebhook(webhook.EventPullFailed, ct.Name, ct.Image, "", result.Error)

		if result.RolledBack {
			// pre-rollback hook
			w.Hooks.RunSoft(hooks.HookPreRollback, ct.Name, ct.Image)

			w.Notifier.NotifyRollback(ct.Name, result.OldImage, result.NewImage)
			w.sendWebhook(webhook.EventRollbackDone, ct.Name, ct.Image, result.OldImage, nil)

			if w.Metrics != nil {
				w.Metrics.RecordRollback()
			}

			// post-rollback hook
			w.Hooks.RunSoft(hooks.HookPostRollback, ct.Name, ct.Image)
		}

		return "failed"
	}

	if result.Updated {
		if w.Metrics != nil {
			w.Metrics.RecordUpdate()
			if !result.NoPull {
				w.Metrics.RecordPull()
			}
		}

		w.Notifier.NotifySuccess(ct.Name, result.OldImage, result.NewImage)
		w.sendWebhook(webhook.EventRecreateSuccess, ct.Name, ct.Image, result.OldImage, nil)

		if w.Config.RemoveAnonymousVols {
			if err := w.Client.RemoveContainerWithVolumes(ct.ID); err != nil {
				logger.Log.Warnf("Failed to remove volumes for %s: %v", ct.Name, err)
			}
		}
		w.Updater.CleanupOldImage(ct.ImageID)

		// post-update hook
		w.Hooks.RunSoft(hooks.HookPostUpdate, ct.Name, ct.Image)

		return "updated"
	}

	return "skipped"
}

// RunContainerUpdate runs update for a specific container by name
func (w *Watcher) RunContainerUpdate(containerName string) error {
	containers, err := w.Client.ListContainersWithOptions(
		w.Config.IncludeStopped,
		w.Config.IncludeRestarting,
	)
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}

	for _, ct := range containers {
		if ct.Name == containerName {
			filtered := Filter([]docker.ContainerInfo{ct}, w.Config)
			if len(filtered) == 0 {
				return fmt.Errorf("container %s exists but is not watched by sentinel", containerName)
			}

			result := w.CheckContainer(ct)
			if result == "failed" {
				return fmt.Errorf("update failed for container %s", containerName)
			}

			logger.Log.Infof("Container update result for %s: %s", containerName, result)
			return nil
		}
	}

	return fmt.Errorf("container %s not found", containerName)
}

// sendWebhook sends a webhook event safely
func (w *Watcher) sendWebhook(
	event webhook.EventType,
	container, image, oldImage string,
	err error,
) {
	if w.Webhook == nil {
		return
	}

	var sendErr error
	switch {
	case err != nil:
		sendErr = w.Webhook.SendWithError(event, container, image, err)
	case oldImage != "":
		sendErr = w.Webhook.SendWithImages(event, container, oldImage, image)
	default:
		sendErr = w.Webhook.Send(event, container, image)
	}

	if sendErr != nil {
		logger.Log.WithFields(logger.Fields{
			"event":     event,
			"container": container,
		}).Warnf("Webhook send failed: %v", sendErr)
	}
}