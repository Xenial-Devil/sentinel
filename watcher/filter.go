package watcher

import (
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
	"strings"
)

const (
	LabelEnableLegacy      = "sentinel.enable"
	LabelEnableCompose     = "com.sentinel.watch.enable"
	LabelEnableComposeTypo = "com.swntinel.watch.enable"
	LabelDisable           = "sentinel.disable"
	LabelMonitorOnly       = "sentinel.monitor-only"
	LabelScope             = "sentinel.scope"
	LabelNoPull            = "sentinel.no-pull"
	LabelNoRestart         = "sentinel.no-restart"
	LabelRollingRestart    = "sentinel.rolling-restart"
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
	// Rule 1: explicit exclude list wins everything
	if isExcludedByName(ct.Name, cfg.ExcludeContainers) {
		logger.Log.Debugf("Container %s skipped: in exclude list", ct.Name)
		return false
	}

	// Rule 2: sentinel.disable=true wins
	if isDisabled(ct) {
		logger.Log.Debugf("Container %s skipped: sentinel.disable=true", ct.Name)
		return false
	}

	// Rule 3: explicit include list - if set, only these names are watched
	if len(cfg.IncludeContainers) > 0 {
		if !isIncludedByName(ct.Name, cfg.IncludeContainers) {
			logger.Log.Debugf("Container %s skipped: not in include list", ct.Name)
			return false
		}
		// In include list - still check scope but skip label check
		if cfg.Scope != "" && GetScope(ct) != cfg.Scope {
			logger.Log.Debugf("Container %s skipped: scope mismatch", ct.Name)
			return false
		}
		return true
	}

	// Rule 4: scope filter
	if cfg.Scope != "" {
		containerScope := GetScope(ct)
		if containerScope != cfg.Scope {
			logger.Log.Debugf("Container %s skipped: scope mismatch (want=%s got=%s)",
				ct.Name, cfg.Scope, containerScope)
			return false
		}
	}

	// Rule 5: WatchAll
	if cfg.WatchAll {
		logger.Log.Debugf("Container %s watched: WatchAll=true", ct.Name)
		return true
	}

	// Rule 6: configured label
	if cfg.LabelEnable && isEnabledByConfiguredLabel(ct, cfg.WatchLabel, cfg.WatchLabelValue) {
		logger.Log.Debugf("Container %s watched: label %s=%s",
			ct.Name, cfg.WatchLabel, cfg.WatchLabelValue)
		return true
	}

	// Rule 7: legacy label
	if isEnabled(ct) {
		logger.Log.Debugf("Container %s watched: sentinel.enable=true", ct.Name)
		return true
	}

	logger.Log.Debugf("Container %s skipped: no watch label found", ct.Name)
	return false
}

// isExcludedByName checks if container name is in exclude list
func isExcludedByName(name string, excludeList []string) bool {
	for _, excluded := range excludeList {
		if strings.EqualFold(name, strings.TrimSpace(excluded)) {
			return true
		}
	}
	return false
}

// isIncludedByName checks if container name is in include list
func isIncludedByName(name string, includeList []string) bool {
	for _, included := range includeList {
		if strings.EqualFold(name, strings.TrimSpace(included)) {
			return true
		}
	}
	return false
}

func isEnabled(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelEnableLegacy, "true")
}

func isEnabledByConfiguredLabel(ct docker.ContainerInfo, labelKey string, labelValue string) bool {
	if labelMatches(ct, labelKey, labelValue) {
		return true
	}
	if labelKey == LabelEnableCompose {
		return labelMatches(ct, LabelEnableComposeTypo, labelValue)
	}
	return false
}

func isDisabled(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelDisable, "true")
}

// IsMonitorOnly checks per-container monitor-only label
func IsMonitorOnly(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelMonitorOnly, "true")
}

// IsNoPull checks if container has no-pull label
func IsNoPull(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelNoPull, "true")
}

// IsNoRestart checks if container has no-restart label
func IsNoRestart(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelNoRestart, "true")
}

// IsRollingRestart checks if container has rolling-restart label
func IsRollingRestart(ct docker.ContainerInfo) bool {
	return labelMatches(ct, LabelRollingRestart, "true")
}

// GetScope returns the scope label value
func GetScope(ct docker.ContainerInfo) string {
	val, ok := ct.Labels[LabelScope]
	if !ok {
		return ""
	}
	return val
}

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