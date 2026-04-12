package watcher

import (
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
)

const (
	// Labels for container control
	LabelEnable      = "sentinel.enable"
	LabelDisable     = "sentinel.disable"
	LabelMonitorOnly = "sentinel.monitor-only"
	LabelScope       = "sentinel.scope"
)

// Filter returns containers that sentinel should watch
func Filter(containers []docker.ContainerInfo, cfg *config.Config) []docker.ContainerInfo {
	var filtered []docker.ContainerInfo

	for _, ct := range containers {
		if shouldWatch(ct, cfg) {
			filtered = append(filtered, ct)
		}
	}

	return filtered
}

// shouldWatch decides if a container should be watched
func shouldWatch(ct docker.ContainerInfo, cfg *config.Config) bool {
	// Check if explicitly disabled by label
	if isDisabled(ct) {
		logger.Log.Debugf("Container %s is disabled by label", ct.Name)
		return false
	}

	// If watch all is enabled
	if cfg.WatchAll {
		return true
	}

	// Check if explicitly enabled by label
	if isEnabled(ct) {
		return true
	}

	// Default - dont watch
	return false
}

// isEnabled checks if container has sentinel.enable label
func isEnabled(ct docker.ContainerInfo) bool {
	val, ok := ct.Labels[LabelEnable]
	if !ok {
		return false
	}
	return val == "true"
}

// isDisabled checks if container has sentinel.disable label
func isDisabled(ct docker.ContainerInfo) bool {
	val, ok := ct.Labels[LabelDisable]
	if !ok {
		return false
	}
	return val == "true"
}

// IsMonitorOnly checks if container should only be monitored
func IsMonitorOnly(ct docker.ContainerInfo) bool {
	val, ok := ct.Labels[LabelMonitorOnly]
	if !ok {
		return false
	}
	return val == "true"
}

// GetScope returns the scope label value
func GetScope(ct docker.ContainerInfo) string {
	val, ok := ct.Labels[LabelScope]
	if !ok {
		return ""
	}
	return val
}