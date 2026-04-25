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
//  1. REPO_USER / REPO_PASS  — generic credentials from .env (applies to all private registries)
//  2. SENTINEL_REGISTRY_USER_<HOST> / SENTINEL_REGISTRY_PASS_<HOST>  — per-registry override
//     e.g. SENTINEL_REGISTRY_USER_GHCR_IO / SENTINEL_REGISTRY_PASS_GHCR_IO
//  3. Docker config.json  — directory resolved from DOCKER_CONFIG env var, then OS default
func GetCredentials(reg string) (*Credentials, error) {
	// 1. Generic .env credentials (REPO_USER / REPO_PASS)
	if creds := getGenericEnvCredentials(); creds != nil {
		logger.Log.Debugf("Using REPO_USER/REPO_PASS credentials for registry: %s", reg)
		return creds, nil
	}

	// 2. Per-registry env vars: SENTINEL_REGISTRY_USER_<NORMALIZED_HOST>
	if creds := getPerRegistryEnvCredentials(reg); creds != nil {
		logger.Log.Debugf("Using per-registry env-var credentials for: %s", reg)
		return creds, nil
	}

	// 3. ~/.docker/config.json (or DOCKER_CONFIG dir)
	return getDockerConfigCredentials(reg)
}

// GetRegistrySpecificCredentials loads credentials for a registry WITHOUT
// falling back to the generic REPO_USER/REPO_PASS variables.
// Use this for Docker Hub so that credentials meant for other private
// registries (GHCR etc.) are never silently forwarded to Docker Hub.
//
// Priority order:
//  1. SENTINEL_REGISTRY_USER_<HOST> / SENTINEL_REGISTRY_PASS_<HOST>
//  2. Docker config.json entry for that registry
func GetRegistrySpecificCredentials(reg string) (*Credentials, error) {
	// 1. Per-registry env vars only
	if creds := getPerRegistryEnvCredentials(reg); creds != nil {
		logger.Log.Debugf("Using per-registry env-var credentials for: %s", reg)
		return creds, nil
	}

	// 2. ~/.docker/config.json (or DOCKER_CONFIG dir)
	return getDockerConfigCredentials(reg)
}

// getGenericEnvCredentials reads the simple REPO_USER / REPO_PASS variables
// that are typically set in a .env file and apply to all private registries.
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
// Normalization: uppercase, '.' ':' '-' replaced with '_'
// Example: ghcr.io → SENTINEL_REGISTRY_USER_GHCR_IO
func getPerRegistryEnvCredentials(reg string) *Credentials {
	normalized := strings.ToUpper(reg)
	normalized = strings.NewReplacer(".", "_", ":", "_", "-", "_").Replace(normalized)

	user := os.Getenv("SENTINEL_REGISTRY_USER_" + normalized)
	pass := os.Getenv("SENTINEL_REGISTRY_PASS_" + normalized)
	if user != "" && pass != "" {
		return &Credentials{Username: user, Password: pass}
	}

	// Single-token variant (e.g. GitHub PAT): SENTINEL_REGISTRY_TOKEN_<HOST>
	if token := os.Getenv("SENTINEL_REGISTRY_TOKEN_" + normalized); token != "" {
		return &Credentials{Username: "token", Password: token}
	}

	return nil
}

// getDockerConfigCredentials reads credentials from Docker's config.json.
// The config directory is resolved via:
//  1. DOCKER_CONFIG env var (standard Docker convention — value is a directory path)
//  2. OS default: ~/.docker/
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
// Honors the DOCKER_CONFIG env var (directory path), otherwise uses the OS default.
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
	return EncodeAuthHeader(creds)
}

// EncodeAuthHeader returns a base64-encoded JSON auth string from Credentials
func EncodeAuthHeader(creds *Credentials) string {
	if creds == nil {
		return ""
	}
	jsonAuth := fmt.Sprintf(`{"username":%q,"password":%q}`, creds.Username, creds.Password)
	return base64.URLEncoding.EncodeToString([]byte(jsonAuth))
}

// GetBasicAuthHeader returns a standard HTTP Basic Authorization header value.
func GetBasicAuthHeader(reg string) string {
	creds, err := GetCredentials(reg)
	if err != nil || creds == nil {
		return ""
	}
	return GetBasicAuthHeaderFromCreds(creds)
}

// GetBasicAuthHeaderFromCreds returns a standard HTTP Basic Authorization header value from Credentials.
func GetBasicAuthHeaderFromCreds(creds *Credentials) string {
	if creds == nil {
		return ""
	}
	raw := creds.Username + ":" + creds.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// splitCredentials splits a "username:password" string on the first colon.
func splitCredentials(auth string) []string {
	for i, c := range auth {
		if c == ':' {
			return []string{auth[:i], auth[i+1:]}
		}
	}
	return nil
}