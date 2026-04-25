package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sentinel/logger"
	"strings"
	"time"
)

// dockerHubTokenURL and dockerHubManifestBase are overridable in tests.
var (
	dockerHubTokenURL    = "https://auth.docker.io/token"
	dockerHubManifestBase = "https://registry-1.docker.io/v2"
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
func (c *Client) GetRemoteDigest(image string, customCreds *Credentials) (string, error) {
	ref := ParseImageRef(image)

	logger.Log.Debugf("Checking registry: registry=%s name=%s tag=%s",
		ref.Registry, ref.Name, ref.Tag)

	// Docker Hub needs token auth
	if strings.Contains(ref.Registry, "docker.io") {
		return c.getDockerHubDigest(ref, customCreds)
	}

	// Private registry - attempt v2 without auth first
	return c.getPrivateDigest(ref, customCreds)
}

// getDockerHubDigest gets digest from Docker Hub using token auth.
//
// Strategy:
//  1. Try with an anonymous token — works for public images within the rate limit.
//  2. On 401 (rate-limited or private image), look for Docker Hub-specific
//     credentials (SENTINEL_REGISTRY_USER_REGISTRY_1_DOCKER_IO or a
//     ~/.docker/config.json entry for docker.io).
//     NOTE: the generic REPO_USER/REPO_PASS are intentionally skipped here
//     because those credentials belong to other private registries (e.g. GHCR)
//     and must not be forwarded to Docker Hub.
func (c *Client) getDockerHubDigest(ref ImageRef, customCreds *Credentials) (string, error) {
	manifestURL := fmt.Sprintf("%s/%s/manifests/%s",
		dockerHubManifestBase, ref.Name, ref.Tag)

	if customCreds == nil {
		// 1. Anonymous token attempt
		anonToken, err := getDockerHubToken(ref.Name, c.HTTPClient, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get docker hub token: %v", err)
		}

		digest, err := c.fetchDigest(manifestURL, anonToken)
		if err == nil {
			return digest, nil
		}

		// 2. Anonymous failed (rate-limited or private) — try Docker Hub-specific creds only
		logger.Log.Debugf("Anonymous Docker Hub fetch failed for %s: %v — trying docker.io-specific credentials", ref.Name, err)
	}

	// Use GetRegistrySpecificCredentials so generic REPO_USER/REPO_PASS (meant for
	// GHCR etc.) are never forwarded to Docker Hub.
	var creds *Credentials
	if customCreds != nil {
		creds = customCreds
	} else {
		var _ error
		creds, _ = GetRegistrySpecificCredentials(ref.Registry)
		if creds == nil {
			// Also check "docker.io" alias used in ~/.docker/config.json
			creds, _ = GetRegistrySpecificCredentials("docker.io")
		}
	}
	if creds == nil {
		return "", fmt.Errorf("registry returned 401 unauthorized and no docker.io credentials are configured")
	}

	logger.Log.Debugf("Retrying Docker Hub fetch for %s with registry-specific credentials", ref.Name)
	authedToken, err := getDockerHubToken(ref.Name, c.HTTPClient, creds)
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated docker hub token: %v", err)
	}

	return c.fetchDigest(manifestURL, authedToken)
}

// getPrivateDigest gets digest from a private registry.
// It first tries anonymous access; on 401 it attempts to authenticate
// using credentials from env vars or ~/.docker/config.json.
func (c *Client) getPrivateDigest(ref ImageRef, customCreds *Credentials) (string, error) {
	scheme := "https"
	// Use http for localhost or plain IP with port
	if ref.Registry == "localhost" || strings.HasPrefix(ref.Registry, "127.") {
		scheme = "http"
	}

	manifestURL := fmt.Sprintf(
		"%s://%s/v2/%s/manifests/%s",
		scheme, ref.Registry, ref.Name, ref.Tag,
	)

	var digest string
	var err error

	if customCreds == nil {
		// 1. Try anonymous first
		digest, err = c.fetchDigest(manifestURL, "")
		if err == nil {
			return digest, nil
		}

		logger.Log.Debugf("Anonymous fetch failed for %s: %v — trying authenticated", ref.Registry, err)
	}

	// 2. Load credentials for this registry
	var creds *Credentials
	if customCreds != nil {
		creds = customCreds
	} else {
		var credsErr error
		creds, credsErr = GetCredentials(ref.Registry)
		if credsErr != nil {
			logger.Log.Debugf("Failed to load credentials for %s: %v", ref.Registry, credsErr)
		}
	}

	if creds == nil {
		return "", fmt.Errorf("private registry auth not configured for %s", ref.Registry)
	}

	// 3. Try Basic auth
	basicHeader := GetBasicAuthHeaderFromCreds(creds)
	digest, err = c.fetchDigest(manifestURL, basicHeader)
	if err == nil {
		return digest, nil
	}

	// 4. Try Bearer token via WWW-Authenticate challenge (e.g. ghcr.io)
	token, tokenErr := c.getBearerToken(ref, scheme, creds)
	if tokenErr != nil {
		logger.Log.Debugf("Bearer token fetch failed for %s: %v", ref.Registry, tokenErr)
		return "", fmt.Errorf("authentication failed for %s: %v", ref.Registry, err)
	}

	digest, err = c.fetchDigest(manifestURL, "Bearer "+token)
	if err != nil {
		return "", fmt.Errorf("authenticated fetch failed for %s: %v", ref.Registry, err)
	}

	return digest, nil
}

// getBearerToken obtains a Bearer token from the registry's auth service.
// It does a HEAD request without auth, reads the WWW-Authenticate header,
// then fetches a token from the provided realm with the given credentials.
func (c *Client) getBearerToken(ref ImageRef, scheme string, creds *Credentials) (string, error) {
	// Probe the registry /v2/ endpoint to get the WWW-Authenticate header
	probeURL := fmt.Sprintf("%s://%s/v2/", scheme, ref.Registry)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, probeURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close probe response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("expected 401 from /v2/, got %d", resp.StatusCode)
	}

	// Parse: Bearer realm="https://...",service="...",scope="..."
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	realm, service, scope := parseBearerChallenge(wwwAuth, ref)
	if realm == "" {
		return "", fmt.Errorf("no Bearer realm in WWW-Authenticate: %s", wwwAuth)
	}

	// Build token request URL
	tokenURL := fmt.Sprintf("%s?service=%s&scope=%s",
		realm,
		url.QueryEscape(service),
		url.QueryEscape(scope),
	)

	tokenReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	tokenReq.SetBasicAuth(creds.Username, creds.Password)

	tokenResp, err := c.HTTPClient.Do(tokenReq)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := tokenResp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close token response body: %v", err)
		}
	}()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", tokenResp.StatusCode)
	}

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&result); err != nil {
		return "", err
	}

	token := result.Token
	if token == "" {
		token = result.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("empty token from auth service")
	}

	return token, nil
}

// parseBearerChallenge parses the WWW-Authenticate Bearer header.
// Example: Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:user/repo:pull"
func parseBearerChallenge(header string, ref ImageRef) (realm, service, scope string) {
	header = strings.TrimPrefix(header, "Bearer ")

	// Default scope for pulling
	scope = fmt.Sprintf("repository:%s:pull", ref.Name)
	service = ref.Registry

	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch key {
		case "realm":
			realm = val
		case "service":
			service = val
		case "scope":
			scope = val
		}
	}

	return realm, service, scope
}

// fetchDigest performs a HEAD request and extracts the Docker-Content-Digest header.
// authHeader can be a full Authorization header value (e.g. "Bearer <token>" or "Basic <b64>")
// or an empty string for anonymous access.
func (c *Client) fetchDigest(manifestURL string, authHeader string) (string, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodHead,
		manifestURL,
		nil,
	)
	if err != nil {
		return "", err
	}

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
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
func (c *Client) HasUpdate(localDigest string, image string, customCreds *Credentials) (bool, string, error) {
	remoteDigest, err := c.GetRemoteDigest(image, customCreds)
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

// getDockerHubToken gets a Bearer token from Docker Hub.
// If creds are provided, they are sent as Basic auth to the token endpoint so
// that Docker Hub applies the authenticated rate limit (unlimited) instead of
// the anonymous one (100 pulls / 6 h / IP).
func getDockerHubToken(imageName string, client *http.Client, creds *Credentials) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s?service=registry.docker.io&scope=repository:%s:pull",
		dockerHubTokenURL, imageName,
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	if creds != nil {
		logger.Log.Debugf("Using authenticated Docker Hub token request for %s", imageName)
		req.SetBasicAuth(creds.Username, creds.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close auth response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("docker hub token endpoint returned %d", resp.StatusCode)
	}

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