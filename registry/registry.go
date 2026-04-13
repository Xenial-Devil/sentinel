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

// ImageRef holds parsed image reference parts
type ImageRef struct {
	Registry string // e.g. registry-1.docker.io
	Name     string // e.g. library/nginx
	Tag      string // e.g. latest
	Digest   string // e.g. sha256:abc...
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

// ParseImageRef parses an image reference correctly handling:
// nginx                          -> docker hub official
// nginx:1.25                     -> docker hub official with tag
// myuser/myapp:latest            -> docker hub user image
// registry:5000/myapp:latest     -> private registry with port
// registry.example.com/myapp     -> private registry with domain
func ParseImageRef(image string) ImageRef {
	ref := ImageRef{
		Registry: "registry-1.docker.io",
		Tag:      "latest",
	}

	// Split digest if present
	if idx := strings.Index(image, "@"); idx != -1 {
		ref.Digest = image[idx+1:]
		image = image[:idx]
	}

	// Split tag - must find last colon after last slash
	// This handles registry:5000/repo vs repo:tag correctly
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")

	if lastColon > lastSlash {
		// colon is after last slash - it is a tag separator
		ref.Tag = image[lastColon+1:]
		image = image[:lastColon]
	}

	// Now image is name without tag
	// Detect if first component is a registry (contains dot or colon or is localhost)
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && isRegistry(parts[0]) {
		ref.Registry = parts[0]
		ref.Name = parts[1]
	} else {
		// Docker Hub
		ref.Registry = "registry-1.docker.io"
		if strings.Contains(image, "/") {
			ref.Name = image
		} else {
			ref.Name = "library/" + image
		}
	}

	return ref
}

// isRegistry checks if a string looks like a registry hostname
func isRegistry(s string) bool {
	return strings.Contains(s, ".") ||
		strings.Contains(s, ":") ||
		s == "localhost"
}

// GetRemoteDigest gets the digest of an image from its registry
func (c *Client) GetRemoteDigest(image string) (string, error) {
	ref := ParseImageRef(image)

	logger.Log.Debugf("Checking registry: registry=%s name=%s tag=%s",
		ref.Registry, ref.Name, ref.Tag)

	// Docker Hub needs token auth
	if strings.Contains(ref.Registry, "docker.io") {
		return c.getDockerHubDigest(ref)
	}

	// Private registry - attempt v2 without auth first
	return c.getPrivateDigest(ref)
}

// getDockerHubDigest gets digest from Docker Hub using token auth
func (c *Client) getDockerHubDigest(ref ImageRef) (string, error) {
	token, err := getDockerHubToken(ref.Name, c.HTTPClient)
	if err != nil {
		return "", fmt.Errorf("failed to get docker hub token: %v", err)
	}

	url := fmt.Sprintf(
		"https://registry-1.docker.io/v2/%s/manifests/%s",
		ref.Name, ref.Tag,
	)

	return c.fetchDigest(url, token)
}

// getPrivateDigest gets digest from a private registry
func (c *Client) getPrivateDigest(ref ImageRef) (string, error) {
	scheme := "https"
	// Use http for localhost or plain IP with port
	if ref.Registry == "localhost" || strings.HasPrefix(ref.Registry, "127.") {
		scheme = "http"
	}

	url := fmt.Sprintf(
		"%s://%s/v2/%s/manifests/%s",
		scheme, ref.Registry, ref.Name, ref.Tag,
	)

	// Try without auth first
	digest, err := c.fetchDigest(url, "")
	if err != nil {
		logger.Log.Debugf("Private registry unauthenticated fetch failed: %v", err)
		return "", fmt.Errorf("private registry auth not configured for %s", ref.Registry)
	}

	return digest, nil
}

// fetchDigest performs HEAD request and extracts Docker-Content-Digest header
func (c *Client) fetchDigest(url string, token string) (string, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodHead,
		url,
		nil,
	)
	if err != nil {
		return "", err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
	}, ", "))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry request failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close registry response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("registry returned 401 unauthorized")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status: %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest in registry response")
	}

	return digest, nil
}

// HasUpdate checks if a newer image is available
func (c *Client) HasUpdate(localDigest string, image string) (bool, string, error) {
	remoteDigest, err := c.GetRemoteDigest(image)
	if err != nil {
		return false, "", err
	}

	if remoteDigest != localDigest {
		logger.Log.WithFields(logger.Fields{
			"image":         image,
			"local_digest":  localDigest[:min(12, len(localDigest))],
			"remote_digest": remoteDigest[:min(12, len(remoteDigest))],
		}).Info("🆕  Update available")
		return true, remoteDigest, nil
	}

	logger.Log.Debugf("No update for %s", image)
	return false, remoteDigest, nil
}

// getDockerHubToken gets a Bearer token from Docker Hub
func getDockerHubToken(imageName string, client *http.Client) (string, error) {
	url := fmt.Sprintf(
		"https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull",
		imageName,
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
		return "", fmt.Errorf("empty token received from docker hub")
	}

	return result.Token, nil
}

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}