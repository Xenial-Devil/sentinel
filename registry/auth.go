package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sentinel/logger"
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

// GetCredentials loads Docker Hub credentials from config file
func GetCredentials(registry string) (*Credentials, error) {
	// Get Docker config path based on OS
	configPath := getDockerConfigPath()

	logger.Log.Debugf("Loading Docker credentials from: %s", configPath)

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		// No credentials file found - use anonymous
		logger.Log.Debug("No Docker credentials file found - using anonymous")
		return nil, nil
	}

	// Parse config
	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse docker config: %v", err)
	}

	// Find credentials for registry
	entry, ok := cfg.Auths[registry]
	if !ok {
		return nil, nil
	}

	// Decode base64 credentials
	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %v", err)
	}

	// Split username:password
	parts := splitCredentials(string(decoded))
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid credentials format")
	}

	return &Credentials{
		Username: parts[0],
		Password: parts[1],
	}, nil
}

// getDockerConfigPath returns Docker config path based on OS
func getDockerConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("USERPROFILE"), ".docker", "config.json")
	default:
		return filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
	}
}

// splitCredentials splits username:password string
func splitCredentials(auth string) []string {
	for i, c := range auth {
		if c == ':' {
			return []string{auth[:i], auth[i+1:]}
		}
	}
	return nil
}