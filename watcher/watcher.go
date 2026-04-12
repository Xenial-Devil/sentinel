package watcher

import (
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
	"sentinel/scheduler"
	"sentinel/updater"
)

// Watcher monitors Docker containers
type Watcher struct {
	Client    *docker.Client
	Config    *config.Config
	Updater   *updater.Updater
	Scheduler *scheduler.Scheduler
}

// New creates a new Watcher
func New(client *docker.Client, cfg *config.Config) *Watcher {
	w := &Watcher{
		Client:  client,
		Config:  cfg,
		Updater: updater.New(client, cfg),
	}

	// Setup scheduler with RunCycle as the job
	w.Scheduler = scheduler.New(cfg, w.RunCycle)

	return w
}

// Start begins the watch loop
func (w *Watcher) Start() {
	logger.Log.Info("Sentinel is starting...")
	w.Scheduler.Start()
}

// RunCycle runs one full check cycle
func (w *Watcher) RunCycle() {
	logger.Log.Info("Starting check cycle...")

	// Get all containers
	containers, err := w.Client.ListContainers(w.Config.IncludeStopped)
	if err != nil {
		logger.Log.Errorf("Failed to list containers: %v", err)
		return
	}

	// Filter containers
	filtered := Filter(containers, w.Config)
	logger.Log.Infof("Watching %d containers", len(filtered))

	// Check each container
	for _, ct := range filtered {
		w.CheckContainer(ct)
	}

	logger.Log.Info("Check cycle complete")
}

// CheckContainer checks a single container for updates
func (w *Watcher) CheckContainer(ct docker.ContainerInfo) {
	logger.Log.Debugf("Checking container: %s (%s)", ct.Name, ct.Image)

	// Skip sentinel itself
	if ct.Name == "sentinel" {
		logger.Log.Debugf("Skipping sentinel itself")
		return
	}

	// Monitor only mode - just log dont update
	if w.Config.MonitorOnly || IsMonitorOnly(ct) {
		logger.Log.Infof("[MONITOR] Container: %s Image: %s",
			ct.Name,
			ct.Image,
		)
		return
	}

	// Check and update container
	result := w.Updater.CheckAndUpdate(ct)
	if result.Error != nil {
		logger.Log.Errorf("Error processing %s: %v", ct.Name, result.Error)
		return
	}

	if result.Updated {
		logger.Log.Infof("✅ Updated: %s", ct.Name)
	}
}