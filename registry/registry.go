package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sentinel/logger"
	"strings"
	"time"
)

// ImageInfo holds image details from registry
type ImageInfo struct {
	Name   string
	Tag    string
	Digest string
}

// Client handles registry communication
type Client struct {
	HTTPClient *http.Client
}

// New creates a new registry client
func New() *Client {
	return &Client{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ParseImage splits image name into parts
// Example: postgres:17-alpine -> name=postgres tag=17-alpine
func ParseImage(image string) (string, string) {
	// Handle images with registry prefix
	// Example: docker.io/library/postgres:17-alpine
	parts := strings.Split(image, ":")

	name := parts[0]
	tag := "latest"

	if len(parts) > 1 {
		tag = parts[1]
	}

	return name, tag
}

// GetRemoteDigest gets the digest of an image from registry
func (c *Client) GetRemoteDigest(image string) (string, error) {
	name, tag := ParseImage(image)

	// Clean up name for Docker Hub
	registryName := cleanImageName(name)

	// Build registry URL
	url := fmt.Sprintf(
		"https://registry-1.docker.io/v2/%s/manifests/%s",
		registryName,
		tag,
	)

	logger.Log.Debugf("Checking registry: %s", url)

	// Get auth token first
	token, err := getAuthToken(registryName, c.HTTPClient)
	if err != nil {
		logger.Log.Errorf("Failed to get auth token: %v", err)
		return "", err
	}

	// Create request
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodHead,
		url,
		nil,
	)
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logger.Log.Errorf("Failed to reach registry: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status: %d", resp.StatusCode)
	}

	// Get digest from header
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest in response")
	}

	logger.Log.Debugf("Remote digest for %s: %s", image, digest)
	return digest, nil
}

// HasUpdate checks if a newer image is available
func (c *Client) HasUpdate(localDigest string, image string) (bool, string, error) {
	remoteDigest, err := c.GetRemoteDigest(image)
	if err != nil {
		return false, "", err
	}

	// Compare digests
	if remoteDigest != localDigest {
		logger.Log.Infof("Update available for %s", image)
		return true, remoteDigest, nil
	}

	logger.Log.Debugf("No update for %s", image)
	return false, remoteDigest, nil
}

// cleanImageName formats image name for Docker Hub API
func cleanImageName(name string) string {
	// Remove docker.io prefix
	name = strings.TrimPrefix(name, "docker.io/")

	// Add library/ prefix for official images
	// Example: postgres -> library/postgres
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}

	return name
}

// getAuthToken gets a Bearer token from Docker Hub
func getAuthToken(image string, client *http.Client) (string, error) {
	url := fmt.Sprintf(
		"https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull",
		image,
	)

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse token response
	var result struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Token == "" {
		return "", fmt.Errorf("empty token received")
	}

	return result.Token, nil
}