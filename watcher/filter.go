package watcher

import (
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
	"strings"
)

const (
	// Labels for container control
	LabelEnableLegacy      = "sentinel.enable"
	LabelEnableCompose     = "com.sentinel.watch.enable"
	LabelEnableComposeTypo = "com.swntinel.watch.enable"
	LabelDisable           = "sentinel.disable"
	LabelMonitorOnly       = "sentinel.monitor-only"
	LabelScope             = "sentinel.scope"
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
//
// Logic:
//  1. sentinel.disable=true  → always skip, regardless of WatchAll
//  2. WatchAll=true          → watch (disable already handled above)
//  3. WatchAll=false         → only watch if label enables it
func shouldWatch(ct docker.ContainerInfo, cfg *config.Config) bool {
	// Rule 1: sentinel.disable=true always wins, even with WatchAll
	if isDisabled(ct) {
		logger.Log.Debugf("Container %s skipped: sentinel.disable=true", ct.Name)
		return false
	}

	// Rule 2: WatchAll=true → watch everything (disable already handled)
	// com.sentinel.watch.enable=false is ignored here, only sentinel.disable blocks
	if cfg.WatchAll {
		logger.Log.Debugf("Container %s watched: WatchAll=true", ct.Name)
		return true
	}

	// Rule 3: WatchAll=false → must have an enable label

	// Check configured label (e.g. com.sentinel.watch.enable=true)
	if cfg.LabelEnable && isEnabledByConfiguredLabel(ct, cfg.WatchLabel, cfg.WatchLabelValue) {
		logger.Log.Debugf("Container %s watched: label %s=%s", ct.Name, cfg.WatchLabel, cfg.WatchLabelValue)
		return true
	}

	// Check legacy label (sentinel.enable=true)
	if isEnabled(ct) {
		logger.Log.Debugf("Container %s watched: sentinel.enable=true", ct.Name)
		return true
	}

	// No matching label found
	logger.Log.Debugf("Container %s skipped: no watch label found", ct.Name)
	return false
}

// isEnabled checks if container has legacy sentinel.enable=true label
func isEnabled(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelEnableLegacy, "true")
}

// isEnabledByConfiguredLabel checks a configurable watch label/value pair.
// Also accepts the common typo key com.swntinel.watch.enable for compatibility.
func isEnabledByConfiguredLabel(ct docker.ContainerInfo, labelKey string, labelValue string) bool {
	if labelMatches(ct, labelKey, labelValue) {
		return true
	}

	// Also check typo variant if using default compose label
	if labelKey == LabelEnableCompose {
		return labelMatches(ct, LabelEnableComposeTypo, labelValue)
	}

	return false
}

// isDisabled checks if container has sentinel.disable=true label
func isDisabled(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelDisable, "true")
}

// IsMonitorOnly checks if container should only be monitored
func IsMonitorOnly(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelMonitorOnly, "true")
}

// GetScope returns the scope label value
func GetScope(ct docker.ContainerInfo) string {
	val, ok := ct.Labels[LabelScope]
	if !ok {
		return ""
	}
	return val
}

// labelMatches checks if a container label matches the expected value (case-insensitive)
func labelMatches(ct docker.ContainerInfo, key string, expected string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}

	value, ok := ct.Labels[key]
	if !ok {
		return false
	}

	if strings.TrimSpace(expected) == "" {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected))
}