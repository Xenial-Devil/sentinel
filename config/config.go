package config

import (
	"os"
	"runtime"
	"strconv"
)

// Config holds all sentinel settings
type Config struct {
	// Docker settings
	DockerHost      string
	DockerTLSVerify bool
	DockerCertPath  string

	// Watcher settings
	PollInterval   int
	CronSchedule   string
	WatchAll       bool
	MonitorOnly    bool
	IncludeStopped bool

	// Update settings
	Cleanup        bool
	RollingRestart bool
	StopTimeout    int

	// Safety settings
	EnableRollback  bool
	HealthTimeout   int
	SemverPolicy    string
	ApprovalEnabled bool

	// Notification settings
	NotifyURL    string
	SlackWebhook string
	EmailTo      string

	// API settings
	APIEnabled bool
	APIPort    int
	APIToken   string

	// Logging settings
	LogLevel  string
	LogFormat string

	// Metrics settings
	MetricsEnabled bool
	MetricsPort    int

	// Webhook settings
    WebhookURL    string
    WebhookSecret string
	// Approval settings
	ApprovalFilePath string
}

// getDefaultDockerHost auto detects OS and returns correct socket
func getDefaultDockerHost() string {
	switch runtime.GOOS {
	case "windows":
		return "npipe:////./pipe/docker_engine"
	case "darwin":
		return "unix:///var/run/docker.sock"
	case "linux":
		return "unix:///var/run/docker.sock"
	default:
		return "unix:///var/run/docker.sock"
	}
}

// Load reads config from environment variables
func Load() *Config {
	return &Config{
		// Docker settings
		DockerHost:      getEnv("SENTINEL_DOCKER_HOST", getDefaultDockerHost()),
		DockerTLSVerify: getBoolEnv("SENTINEL_TLS_VERIFY", false),
		DockerCertPath:  getEnv("SENTINEL_CERT_PATH", ""),

		// Watcher settings
		PollInterval:   getIntEnv("SENTINEL_POLL_INTERVAL", 30),
		CronSchedule:   getEnv("SENTINEL_CRON", ""),
		WatchAll:       getBoolEnv("SENTINEL_WATCH_ALL", true),
		MonitorOnly:    getBoolEnv("SENTINEL_MONITOR_ONLY", false),
		IncludeStopped: getBoolEnv("SENTINEL_INCLUDE_STOPPED", false),

		// Update settings
		Cleanup:        getBoolEnv("SENTINEL_CLEANUP", true),
		RollingRestart: getBoolEnv("SENTINEL_ROLLING_RESTART", false),
		StopTimeout:    getIntEnv("SENTINEL_STOP_TIMEOUT", 10),

		// Safety settings
		EnableRollback:  getBoolEnv("SENTINEL_ROLLBACK", true),
		HealthTimeout:   getIntEnv("SENTINEL_HEALTH_TIMEOUT", 30),
		SemverPolicy:    getEnv("SENTINEL_SEMVER_POLICY", "all"),
		ApprovalEnabled: getBoolEnv("SENTINEL_APPROVAL", false),

		// Notification settings
		NotifyURL:    getEnv("SENTINEL_NOTIFY_URL", ""),
		SlackWebhook: getEnv("SENTINEL_SLACK_WEBHOOK", ""),
		EmailTo:      getEnv("SENTINEL_EMAIL_TO", ""),

		// API settings
		APIEnabled: getBoolEnv("SENTINEL_API_ENABLED", true),
		APIPort:    getIntEnv("SENTINEL_API_PORT", 8080),
		APIToken:   getEnv("SENTINEL_API_TOKEN", ""),

		// Logging settings
		LogLevel:  getEnv("SENTINEL_LOG_LEVEL", "info"),
		LogFormat: getEnv("SENTINEL_LOG_FORMAT", "pretty"),

		// Metrics settings
		MetricsEnabled: getBoolEnv("SENTINEL_METRICS_ENABLED", true),
		MetricsPort:    getIntEnv("SENTINEL_METRICS_PORT", 9090),
		// Webhook settings
		WebhookURL:    getEnv("SENTINEL_WEBHOOK_URL", ""),
		WebhookSecret: getEnv("SENTINEL_WEBHOOK_SECRET", ""),
		// Approval settings
		ApprovalFilePath: getEnv("SENTINEL_APPROVAL_FILE", "approvals.json"),
		
	}
}

// helper: get string env
func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// helper: get bool env
func getBoolEnv(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return b
	}
	return fallback
}

// helper: get int env
func getIntEnv(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return i
	}
	return fallback
}