package config

import (
	"os"
	"runtime"
	"strconv"
)

// Config holds all sentinel settings
type Config struct {
	// Docker settings
	DockerHost        string
	DockerTLSVerify   bool
	DockerCertPath    string

	// Watcher settings
	PollInterval      int
	CronSchedule      string
	WatchAll          bool
	LabelEnable       bool
	WatchLabel        string
	WatchLabelValue   string
	MonitorOnly       bool
	IncludeStopped    bool
	IncludeRestarting bool
	ReviveStopped     bool
	Scope             string

	// Include/Exclude by name
	IncludeContainers []string
	ExcludeContainers []string

	// Update settings
	Cleanup             bool
	RollingRestart      bool
	StopTimeout         int
	NoPull              bool
	NoRestart           bool
	RemoveAnonymousVols bool
	RunOnce             bool

	// Safety settings
	EnableRollback  bool
	HealthTimeout   int
	SemverPolicy    string
	ApprovalEnabled bool

	// Lifecycle hooks (global - applies to all containers)
	HookPreCheck     string
	HookPostCheck    string
	HookPreUpdate    string
	HookPostUpdate   string
	HookPreRollback  string
	HookPostRollback string
	HookTimeout      int

	// Notification settings
	NotifyURL       string
	SlackWebhook    string
	TeamsWebhook    string
	EmailTo         string
	EmailFrom       string
	EmailHost       string
	EmailPort       string
	EmailUsername   string
	EmailPassword   string

	// Custom notification templates per event
	// Key: event name (e.g. "update_success"), Value: template string
	NotifyTemplates map[string]string

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

func getDefaultDockerHost() string {
	switch runtime.GOOS {
	case "windows":
		return "npipe:////./pipe/docker_engine"
	default:
		return "unix:///var/run/docker.sock"
	}
}

// Load reads config from environment variables
func Load() *Config {
	return &Config{
		DockerHost:        getEnv("SENTINEL_DOCKER_HOST", getDefaultDockerHost()),
		DockerTLSVerify:   getBoolEnv("SENTINEL_TLS_VERIFY", false),
		DockerCertPath:    getEnv("SENTINEL_CERT_PATH", ""),

		PollInterval:      getIntEnv("SENTINEL_POLL_INTERVAL", 30),
		CronSchedule:      getEnv("SENTINEL_CRON", ""),
		WatchAll:          getBoolEnv("SENTINEL_WATCH_ALL", false),
		LabelEnable:       getBoolEnv("SENTINEL_LABEL_ENABLE", true),
		WatchLabel:        getEnv("SENTINEL_WATCH_LABEL", "com.sentinel.watch.enable"),
		WatchLabelValue:   getEnv("SENTINEL_WATCH_LABEL_VALUE", "true"),
		MonitorOnly:       getBoolEnv("SENTINEL_MONITOR_ONLY", false),
		IncludeStopped:    getBoolEnv("SENTINEL_INCLUDE_STOPPED", false),
		IncludeRestarting: getBoolEnv("SENTINEL_INCLUDE_RESTARTING", false),
		ReviveStopped:     getBoolEnv("SENTINEL_REVIVE_STOPPED", false),
		Scope:             getEnv("SENTINEL_SCOPE", ""),

		IncludeContainers: getStringSliceEnv("SENTINEL_INCLUDE_CONTAINERS"),
		ExcludeContainers: getStringSliceEnv("SENTINEL_EXCLUDE_CONTAINERS"),

		Cleanup:             getBoolEnv("SENTINEL_CLEANUP", true),
		RollingRestart:      getBoolEnv("SENTINEL_ROLLING_RESTART", false),
		StopTimeout:         getIntEnv("SENTINEL_STOP_TIMEOUT", 10),
		NoPull:              getBoolEnv("SENTINEL_NO_PULL", false),
		NoRestart:           getBoolEnv("SENTINEL_NO_RESTART", false),
		RemoveAnonymousVols: getBoolEnv("SENTINEL_REMOVE_VOLUMES", false),
		RunOnce:             getBoolEnv("SENTINEL_RUN_ONCE", false),

		EnableRollback:  getBoolEnv("SENTINEL_ROLLBACK", true),
		HealthTimeout:   getIntEnv("SENTINEL_HEALTH_TIMEOUT", 30),
		SemverPolicy:    getEnv("SENTINEL_SEMVER_POLICY", "all"),
		ApprovalEnabled: getBoolEnv("SENTINEL_APPROVAL", false),

		// Lifecycle hooks
		HookPreCheck:     getEnv("SENTINEL_HOOK_PRE_CHECK", ""),
		HookPostCheck:    getEnv("SENTINEL_HOOK_POST_CHECK", ""),
		HookPreUpdate:    getEnv("SENTINEL_HOOK_PRE_UPDATE", ""),
		HookPostUpdate:   getEnv("SENTINEL_HOOK_POST_UPDATE", ""),
		HookPreRollback:  getEnv("SENTINEL_HOOK_PRE_ROLLBACK", ""),
		HookPostRollback: getEnv("SENTINEL_HOOK_POST_ROLLBACK", ""),
		HookTimeout:      getIntEnv("SENTINEL_HOOK_TIMEOUT", 30),

		// Notifications
		NotifyURL:     getEnv("SENTINEL_NOTIFY_URL", ""),
		SlackWebhook:  getEnv("SENTINEL_SLACK_WEBHOOK", ""),
		TeamsWebhook:  getEnv("SENTINEL_TEAMS_WEBHOOK", ""),
		EmailTo:       getEnv("SENTINEL_EMAIL_TO", ""),
		EmailFrom:     getEnv("SENTINEL_EMAIL_FROM", "sentinel@localhost"),
		EmailHost:     getEnv("SENTINEL_EMAIL_HOST", "smtp.gmail.com"),
		EmailPort:     getEnv("SENTINEL_EMAIL_PORT", "587"),
		EmailUsername: getEnv("SENTINEL_EMAIL_USERNAME", ""),
		EmailPassword: getEnv("SENTINEL_EMAIL_PASSWORD", ""),

		// Custom templates
		NotifyTemplates: loadNotifyTemplates(),

		APIEnabled: getBoolEnv("SENTINEL_API_ENABLED", true),
		APIPort:    getIntEnv("SENTINEL_API_PORT", 8080),
		APIToken:   getEnv("SENTINEL_API_TOKEN", ""),

		LogLevel:  getEnv("SENTINEL_LOG_LEVEL", "info"),
		LogFormat: getEnv("SENTINEL_LOG_FORMAT", "pretty"),

		MetricsEnabled: getBoolEnv("SENTINEL_METRICS_ENABLED", true),
		MetricsPort:    getIntEnv("SENTINEL_METRICS_PORT", 9090),

		WebhookURL:    getEnv("SENTINEL_WEBHOOK_URL", ""),
		WebhookSecret: getEnv("SENTINEL_WEBHOOK_SECRET", ""),

		ApprovalFilePath: getEnv("SENTINEL_APPROVAL_FILE", "approvals.json"),
	}
}

// loadNotifyTemplates loads custom templates from env vars
// e.g. SENTINEL_TEMPLATE_UPDATE_SUCCESS="✅ {{.Container}} updated to {{.NewImage}}"
func loadNotifyTemplates() map[string]string {
	templates := make(map[string]string)

	eventEnvMap := map[string]string{
		"update_found":   "SENTINEL_TEMPLATE_UPDATE_FOUND",
		"update_success": "SENTINEL_TEMPLATE_UPDATE_SUCCESS",
		"update_failed":  "SENTINEL_TEMPLATE_UPDATE_FAILED",
		"rollback":       "SENTINEL_TEMPLATE_ROLLBACK",
		"health_failed":  "SENTINEL_TEMPLATE_HEALTH_FAILED",
		"startup":        "SENTINEL_TEMPLATE_STARTUP",
	}

	for event, envKey := range eventEnvMap {
		if val := getEnv(envKey, ""); val != "" {
			templates[event] = val
		}
	}

	return templates
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

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

func getStringSliceEnv(key string) []string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return []string{}
	}
	var result []string
	for _, item := range splitAndTrim(value, ",") {
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func splitAndTrim(s string, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s string, sep string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}