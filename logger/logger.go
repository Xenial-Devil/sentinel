package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Log is the global logger instance
var Log = logrus.New()

// Fields type alias for convenience
type Fields = logrus.Fields

// Init sets up the logger
func Init(level string, format string) {
	setLevel(level)
	setFormat(format)
	Log.SetOutput(os.Stdout)
	Log.SetReportCaller(false)

	Log.WithFields(Fields{
		"level":  level,
		"format": format,
	}).Info("Logger initialized")
}

// ─── Scoped Entry Builders ───────────────────────────────────────────────────

// WithField returns entry with a single field
func WithField(key string, value interface{}) *logrus.Entry {
	return Log.WithField(key, value)
}

// WithFields returns entry with multiple fields
func WithFields(fields Fields) *logrus.Entry {
	return Log.WithFields(fields)
}

// WithError returns entry with error field
func WithError(err error) *logrus.Entry {
	return Log.WithError(err)
}

// WithCaller returns entry with file and line number
func WithCaller() *logrus.Entry {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		return Log.WithField("caller", "unknown")
	}
	return Log.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(file), line))
}

// WithContainer returns entry scoped to a container
func WithContainer(name, image string) *logrus.Entry {
	return Log.WithFields(Fields{
		"container": name,
		"image":     image,
	})
}

// WithContainerFull returns entry scoped to a container with all details
func WithContainerFull(name, image, id, status string) *logrus.Entry {
	return Log.WithFields(Fields{
		"container": name,
		"image":     image,
		"id":        id,
		"status":    status,
	})
}

// WithComponent returns entry scoped to a component
func WithComponent(component string) *logrus.Entry {
	return Log.WithField("component", component)
}

// WithUpdate returns entry scoped to an update operation
func WithUpdate(container, oldImage, newImage string) *logrus.Entry {
	return Log.WithFields(Fields{
		"container": container,
		"old_image": oldImage,
		"new_image": newImage,
	})
}

// WithDuration returns entry with a duration field
func WithDuration(d time.Duration) *logrus.Entry {
	return Log.WithField("duration", FormatDuration(d))
}

// ─── Sentinel Domain Loggers ─────────────────────────────────────────────────

// LogStartup logs sentinel startup banner
func LogStartup(version, dockerHost string) {
	Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	Log.Infof("  🛡  Sentinel - Docker Auto-Updater")
	Log.Infof("  Version     : %s", version)
	Log.Infof("  Docker Host : %s", dockerHost)
	Log.Infof("  PID         : %d", os.Getpid())
	Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// LogShutdown logs sentinel shutdown
func LogShutdown(reason string) {
	Log.Infof("🛑  Sentinel shutting down — reason: %s", reason)
}

// LogCycleStart logs the beginning of a watch cycle
func LogCycleStart(cycleNum int) {
	Log.WithField("cycle", cycleNum).
		Info("🔄  Starting watch cycle")
}

// LogCycleEnd logs the end of a watch cycle with summary
func LogCycleEnd(cycleNum, total, updated, skipped, failed int, elapsed time.Duration) {
	Log.WithFields(Fields{
		"cycle":    cycleNum,
		"total":    total,
		"updated":  updated,
		"skipped":  skipped,
		"failed":   failed,
		"duration": FormatDuration(elapsed),
	}).Infof("✅  Cycle #%d complete — %d checked, %d updated, %d skipped, %d failed in %s",
		cycleNum, total, updated, skipped, failed, FormatDuration(elapsed))
}

// LogContainerFound logs a discovered container
func LogContainerFound(name, image, status string) {
	Log.WithFields(Fields{
		"container": name,
		"image":     image,
		"status":    status,
	}).Debugf("🔍  Found container: %-20s  image=%-30s  status=%s", name, image, status)
}

// LogContainerSkipped logs a skipped container with reason
func LogContainerSkipped(name, image, reason string) {
	Log.WithFields(Fields{
		"container": name,
		"image":     image,
		"reason":    reason,
	}).Debugf("⏭   Skipped: %-20s  reason=%s", name, reason)
}

// LogContainerWatched logs a container being watched
func LogContainerWatched(name, image string) {
	Log.WithFields(Fields{
		"container": name,
		"image":     image,
	}).Debugf("👁   Watching: %-20s  image=%s", name, image)
}

// LogContainerMonitorOnly logs a monitor-only container
func LogContainerMonitorOnly(name, image, currentTag, latestTag string, hasUpdate bool) {
	if hasUpdate {
		Log.WithFields(Fields{
			"container":  name,
			"image":      image,
			"current":    currentTag,
			"latest":     latestTag,
			"has_update": true,
		}).Infof("👁   [MONITOR] %-20s  update available: %s → %s (monitor-only, no action taken)",
			name, currentTag, latestTag)
	} else {
		Log.WithFields(Fields{
			"container": name,
			"image":     image,
			"current":   currentTag,
		}).Infof("👁   [MONITOR] %-20s  image=%s (up to date)", name, image)
	}
}

// LogImageCheckStart logs beginning of image check
func LogImageCheckStart(container, image string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
	}).Debugf("🔎  Checking image: %-20s  image=%s", container, image)
}

// LogImageUpToDate logs that an image is up to date
func LogImageUpToDate(container, image, tag string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"tag":       tag,
	}).Infof("✔   Up to date: %-20s  image=%s  tag=%s", container, image, tag)
}

// LogImageUpdateFound logs that a new image version was found
func LogImageUpdateFound(container, image, currentTag, latestTag string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"current":   currentTag,
		"latest":    latestTag,
	}).Infof("🆕  Update found: %-20s  %s → %s", container, currentTag, latestTag)
}

// LogImagePullStart logs beginning of image pull
func LogImagePullStart(container, image string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
	}).Infof("📥  Pulling image: %-20s  image=%s", container, image)
}

// LogImagePullSuccess logs successful image pull
func LogImagePullSuccess(container, image string, elapsed time.Duration) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"duration":  FormatDuration(elapsed),
	}).Infof("📦  Image pulled: %-20s  image=%s  took=%s", container, image, FormatDuration(elapsed))
}

// LogImagePullFailed logs failed image pull
func LogImagePullFailed(container, image string, err error) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"error":     err,
	}).Errorf("❌  Pull failed: %-20s  image=%s  error=%v", container, image, err)
}

// LogContainerStopping logs container stop
func LogContainerStopping(container, id string, timeout int) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
		"timeout":   timeout,
	}).Infof("⏹   Stopping: %-20s  id=%s  timeout=%ds", container, id, timeout)
}

// LogContainerStopped logs container stopped
func LogContainerStopped(container, id string, elapsed time.Duration) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
		"duration":  FormatDuration(elapsed),
	}).Infof("⏹   Stopped: %-20s  id=%s  took=%s", container, id, FormatDuration(elapsed))
}

// LogContainerRemoving logs container removal
func LogContainerRemoving(container, id string) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
	}).Infof("🗑   Removing: %-20s  id=%s", container, id)
}

// LogContainerRemoved logs container removed
func LogContainerRemoved(container, id string) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
	}).Infof("🗑   Removed: %-20s  id=%s", container, id)
}

// LogContainerStarting logs container start
func LogContainerStarting(container, id string) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
	}).Infof("▶   Starting: %-20s  id=%s", container, id)
}

// LogContainerStarted logs container started
func LogContainerStarted(container, id string, elapsed time.Duration) {
	Log.WithFields(Fields{
		"container": container,
		"id":        id,
		"duration":  FormatDuration(elapsed),
	}).Infof("▶   Started: %-20s  id=%s  took=%s", container, id, FormatDuration(elapsed))
}

// LogUpdateSuccess logs a successful update
func LogUpdateSuccess(container, oldImage, newImage string, elapsed time.Duration) {
	Log.WithFields(Fields{
		"container": container,
		"old_image": oldImage,
		"new_image": newImage,
		"duration":  FormatDuration(elapsed),
	}).Infof("✅  Updated: %-20s  %s → %s  took=%s",
		container, oldImage, newImage, FormatDuration(elapsed))
}

// LogUpdateFailed logs a failed update
func LogUpdateFailed(container, image string, err error) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"error":     err,
	}).Errorf("❌  Update failed: %-20s  image=%s  error=%v", container, image, err)
}

// LogRollbackStart logs rollback beginning
func LogRollbackStart(container, fromImage, toImage string) {
	Log.WithFields(Fields{
		"container":  container,
		"from_image": fromImage,
		"to_image":   toImage,
	}).Warnf("⏪  Rolling back: %-20s  %s → %s", container, fromImage, toImage)
}

// LogRollbackSuccess logs successful rollback
func LogRollbackSuccess(container, image string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
	}).Warnf("⏪  Rolled back: %-20s  restored to %s", container, image)
}

// LogRollbackFailed logs failed rollback
func LogRollbackFailed(container, image string, err error) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"error":     err,
	}).Errorf("💥  Rollback failed: %-20s  image=%s  error=%v", container, image, err)
}

// LogHealthCheck logs health check status
func LogHealthCheck(container string, healthy bool, elapsed time.Duration) {
	if healthy {
		Log.WithFields(Fields{
			"container": container,
			"healthy":   true,
			"duration":  FormatDuration(elapsed),
		}).Infof("💚  Healthy: %-20s  took=%s", container, FormatDuration(elapsed))
	} else {
		Log.WithFields(Fields{
			"container": container,
			"healthy":   false,
			"duration":  FormatDuration(elapsed),
		}).Warnf("💛  Unhealthy: %-20s  took=%s", container, FormatDuration(elapsed))
	}
}

// LogHealthTimeout logs a health check timeout
func LogHealthTimeout(container string, timeout int) {
	Log.WithFields(Fields{
		"container": container,
		"timeout":   timeout,
	}).Errorf("🔴  Health timeout: %-20s  waited %ds with no healthy state", container, timeout)
}

// LogApprovalRequired logs that manual approval is needed
func LogApprovalRequired(container, image, currentTag, latestTag string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
		"current":   currentTag,
		"latest":    latestTag,
	}).Warnf("⏳  Approval required: %-20s  %s → %s  (use API to approve)",
		container, currentTag, latestTag)
}

// LogApprovalGranted logs that an update was approved
func LogApprovalGranted(container, image string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
	}).Infof("✅  Approved: %-20s  image=%s", container, image)
}

// LogApprovalDenied logs that an update was denied
func LogApprovalDenied(container, image string) {
	Log.WithFields(Fields{
		"container": container,
		"image":     image,
	}).Warnf("🚫  Denied: %-20s  image=%s", container, image)
}

// LogCleanup logs image cleanup
func LogCleanup(image string, freed int64) {
	Log.WithFields(Fields{
		"image": image,
		"freed": formatBytes(freed),
	}).Infof("🧹  Cleaned: image=%s  freed=%s", image, formatBytes(freed))
}

// LogNotification logs notification dispatch
func LogNotification(notifyType, target string, success bool) {
	if success {
		Log.WithFields(Fields{
			"type":   notifyType,
			"target": target,
		}).Infof("📣  Notification sent: type=%s  target=%s", notifyType, target)
	} else {
		Log.WithFields(Fields{
			"type":   notifyType,
			"target": target,
		}).Warnf("📣  Notification failed: type=%s  target=%s", notifyType, target)
	}
}

// LogAPIStart logs API server startup
func LogAPIStart(port int, token bool) {
	Log.WithFields(Fields{
		"port":      port,
		"auth":      token,
	}).Infof("🌐  API server started  port=%d  auth=%v", port, token)
}

// LogMetricsStart logs metrics server startup
func LogMetricsStart(port int) {
	Log.WithField("port", port).
		Infof("📊  Metrics server started  port=%d", port)
}

// LogScheduler logs scheduler info
func LogScheduler(mode string, value string) {
	Log.WithFields(Fields{
		"mode":  mode,
		"value": value,
	}).Infof("🕐  Scheduler configured  mode=%s  value=%s", mode, value)
}

// LogDockerConnected logs docker connection
func LogDockerConnected(host, version string) {
	Log.WithFields(Fields{
		"host":    host,
		"version": version,
	}).Infof("🐳  Docker connected  host=%s  version=%s", host, version)
}

// LogDockerDisconnected logs docker disconnection
func LogDockerDisconnected(err error) {
	Log.WithError(err).Error("🐳  Docker connection lost")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// IsDebug returns true if debug level is enabled
func IsDebug() bool {
	return Log.IsLevelEnabled(logrus.DebugLevel)
}

// IsTrace returns true if trace level is enabled
func IsTrace() bool {
	return Log.IsLevelEnabled(logrus.TraceLevel)
}

// GetLevel returns current log level as string
func GetLevel() string {
	return Log.GetLevel().String()
}

// FormatDuration formats a duration into human readable string
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

// formatBytes formats bytes into human readable string
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ─── Internal ─────────────────────────────────────────────────────────────────

func setLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		Log.SetLevel(logrus.TraceLevel)
	case "debug":
		Log.SetLevel(logrus.DebugLevel)
	case "warn", "warning":
		Log.SetLevel(logrus.WarnLevel)
	case "error":
		Log.SetLevel(logrus.ErrorLevel)
	case "fatal":
		Log.SetLevel(logrus.FatalLevel)
	case "panic":
		Log.SetLevel(logrus.PanicLevel)
	default:
		Log.SetLevel(logrus.InfoLevel)
	}
}

func setFormat(format string) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		Log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	case "logfmt":
		Log.SetFormatter(&logrus.TextFormatter{
			DisableColors:   true,
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		Log.SetFormatter(&SentinelFormatter{})
	}
}