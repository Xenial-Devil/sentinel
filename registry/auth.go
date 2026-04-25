package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sentinel/logger"
	"strings"
)

// DockerConfig holds Docker credentials
type DockerConfig struct {
	Auths map[string]AuthEntry `json:"auths"`
}

// AuthEntry holds encoded credentials
type AuthEntry struct {
	Auth string `json:"auth"`
}

// Credentials holds decoded username and password
type Credentials struct {
	Username string
	Password string
}

// GetCredentials loads credentials for a registry.
// Priority order:
//  1. REPO_USER / REPO_PASS         — generic credentials (applies to all registries)
//  2. SENTINEL_REGISTRY_USER_<HOST> — per-registry env var override
//  3. Docker config.json            — from DOCKER_CONFIG env var or OS default
func GetCredentials(reg string) (*Credentials, error) {
	// 1. Generic env credentials (REPO_USER / REPO_PASS)
	if creds := getGenericEnvCredentials(); creds != nil {
		logger.Log.Debugf("Using REPO_USER/REPO_PASS credentials for registry: %s", reg)
		return creds, nil
	}

	// 2. Per-registry env vars
	if creds := getPerRegistryEnvCredentials(reg); creds != nil {
		logger.Log.Debugf("Using per-registry env-var credentials for: %s", reg)
		return creds, nil
	}

	// 3. Docker config.json
	return getDockerConfigCredentials(reg)
}

// getGenericEnvCredentials reads REPO_USER / REPO_PASS variables
func getGenericEnvCredentials() *Credentials {
	user := os.Getenv("REPO_USER")
	pass := os.Getenv("REPO_PASS")
	if user != "" && pass != "" {
		return &Credentials{Username: user, Password: pass}
	}
	return nil
}

// getPerRegistryEnvCredentials reads registry-specific env vars.
// Key format: SENTINEL_REGISTRY_USER_<NORMALIZED_HOST>
// Normalization: uppercase, dots/colons/hyphens replaced with underscores
// Example: ghcr.io → SENTINEL_REGISTRY_USER_GHCR_IO
func getPerRegistryEnvCredentials(reg string) *Credentials {
	normalized := strings.ToUpper(reg)
	normalized = strings.NewReplacer(".", "_", ":", "_", "-", "_").Replace(normalized)

	user := os.Getenv("SENTINEL_REGISTRY_USER_" + normalized)
	pass := os.Getenv("SENTINEL_REGISTRY_PASS_" + normalized)
	if user != "" && pass != "" {
		return &Credentials{Username: user, Password: pass}
	}

	// Single-token variant (e.g. GitHub PAT)
	if token := os.Getenv("SENTINEL_REGISTRY_TOKEN_" + normalized); token != "" {
		return &Credentials{Username: "token", Password: token}
	}

	return nil
}

// getDockerConfigCredentials reads credentials from Docker's config.json.
func getDockerConfigCredentials(reg string) (*Credentials, error) {
	configPath := getDockerConfigPath()
	logger.Log.Debugf("Loading Docker credentials from: %s", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Log.Debug("No Docker credentials file found - using anonymous")
		return nil, nil
	}

	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse docker config: %v", err)
	}

	entry, ok := cfg.Auths[reg]
	if !ok {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %v", err)
	}

	parts := splitCredentials(string(decoded))
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid credentials format")
	}

	return &Credentials{
		Username: parts[0],
		Password: parts[1],
	}, nil
}

// getDockerConfigPath returns the full path to Docker's config.json.
func getDockerConfigPath() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("USERPROFILE"), ".docker", "config.json")
	default:
		return filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
	}
}

// GetAuthHeader returns a base64-encoded JSON auth string for Docker's
// ImagePullOptions.RegistryAuth field. Returns "" if no credentials found.
func GetAuthHeader(reg string) string {
	creds, err := GetCredentials(reg)
	if err != nil || creds == nil {
		return ""
	}
	return buildAuthHeader(creds.Username, creds.Password)
}

// GetAuthHeaderWithCreds returns auth header using label creds if provided,
// otherwise falls back to global credentials.
func GetAuthHeaderWithCreds(reg, labelUser, labelPass string) string {
	// Label creds take priority
	if labelUser != "" && labelPass != "" {
		logger.Log.Debugf("Using label credentials for pull auth header: %s", reg)
		return buildAuthHeader(labelUser, labelPass)
	}
	return GetAuthHeader(reg)
}

// buildAuthHeader builds a base64-encoded JSON auth string
func buildAuthHeader(user, pass string) string {
	jsonAuth := fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)
	return base64.URLEncoding.EncodeToString([]byte(jsonAuth))
}

// GetBasicAuthHeader returns a standard HTTP Basic Authorization header value.
func GetBasicAuthHeader(reg string) string {
	creds, err := GetCredentials(reg)
	if err != nil || creds == nil {
		return ""
	}
	return buildBasicAuthHeader(creds.Username, creds.Password)
}

// buildBasicAuthHeader builds a Basic auth header from raw credentials
func buildBasicAuthHeader(user, pass string) string {
	raw := user + ":" + pass
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// splitCredentials splits "username:password" on the first colon
func splitCredentials(auth string) []string {
	for i, c := range auth {
		if c == ':' {
			return []string{auth[:i], auth[i+1:]}
		}
	}
	return nil
}