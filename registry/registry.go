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
func ParseImage(image string) (string, string) {
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

	registryName := cleanImageName(name)

	url := fmt.Sprintf(
		"https://registry-1.docker.io/v2/%s/manifests/%s",
		registryName,
		tag,
	)

	logger.Log.Debugf("Checking registry: %s", url)

	token, err := getAuthToken(registryName, c.HTTPClient)
	if err != nil {
		logger.Log.Errorf("Failed to get auth token: %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodHead,
		url,
		nil,
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logger.Log.Errorf("Failed to reach registry: %v", err)
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status: %d", resp.StatusCode)
	}

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

	if remoteDigest != localDigest {
		logger.Log.Infof("Update available for %s", image)
		return true, remoteDigest, nil
	}

	logger.Log.Debugf("No update for %s", image)
	return false, remoteDigest, nil
}

// cleanImageName formats image name for Docker Hub API
func cleanImageName(name string) string {
	name = strings.TrimPrefix(name, "docker.io/")

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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close auth response body: %v", err)
		}
	}()

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