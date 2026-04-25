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

var (
	dockerHubTokenURL     = "https://auth.docker.io/token"
	dockerHubManifestBase = "https://registry-1.docker.io/v2"
)

// ImageRef holds parsed image reference parts
type ImageRef struct {
	Registry string
	Name     string
	Tag      string
	Digest   string
}

// RemoteImageInfo holds full remote image information
type RemoteImageInfo struct {
	Digest    string // Docker-Content-Digest from manifest HEAD
	Tag       string // Tag that was checked
	MediaType string // Manifest media type
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
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")

	if lastColon > lastSlash {
		ref.Tag = image[lastColon+1:]
		image = image[:lastColon]
	}

	// Detect if first component is a registry
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && isRegistry(parts[0]) {
		ref.Registry = parts[0]
		ref.Name = parts[1]
	} else {
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

// GetRemoteDigest gets the digest of an image from its registry.
func (c *Client) GetRemoteDigest(image string) (string, error) {
	return c.GetRemoteDigestWithCreds(image, "", "")
}

// GetRemoteDigestWithCreds gets digest with optional label-level credential override.
func (c *Client) GetRemoteDigestWithCreds(image, labelUser, labelPass string) (string, error) {
	info, err := c.GetRemoteImageInfo(image, labelUser, labelPass)
	if err != nil {
		return "", err
	}
	return info.Digest, nil
}

// GetRemoteImageInfo gets full remote image info (digest + tag + media type).
func (c *Client) GetRemoteImageInfo(image, labelUser, labelPass string) (*RemoteImageInfo, error) {
	ref := ParseImageRef(image)

	logger.Log.Debugf("Checking registry: registry=%s name=%s tag=%s",
		ref.Registry, ref.Name, ref.Tag)

	if strings.Contains(ref.Registry, "docker.io") {
		return c.getDockerHubInfo(ref, labelUser, labelPass)
	}

	return c.getPrivateInfo(ref, labelUser, labelPass)
}

// ─────────────────────────────────────────────────────────────────────────────
// Docker Hub
// ─────────────────────────────────────────────────────────────────────────────

// getDockerHubInfo handles Docker Hub images.
// Flow:
//  1. Try anonymous token (works for public images like postgres, nginx)
//  2. If that fails → try with credentials (for private Docker Hub repos)
func (c *Client) getDockerHubInfo(ref ImageRef, labelUser, labelPass string) (*RemoteImageInfo, error) {
	manifestURL := fmt.Sprintf("%s/%s/manifests/%s",
		dockerHubManifestBase, ref.Name, ref.Tag)

	// ── Step 1: Try anonymous public token ───────────────────────────────────
	token, err := getDockerHubToken(ref.Name, "", "", c.HTTPClient)
	if err == nil {
		info, fetchErr := c.fetchManifestInfo(manifestURL, "Bearer "+token)
		if fetchErr == nil {
			info.Tag = ref.Tag
			logger.Log.Debugf("Docker Hub public access succeeded for %s digest=%s",
				ref.Name, shortDigest(info.Digest))
			return info, nil
		}
	}

	logger.Log.Debugf("Docker Hub anonymous access failed for %s — trying authenticated", ref.Name)

	// ── Step 2: Resolve credentials ──────────────────────────────────────────
	user, pass := resolveCredentials(ref.Registry, labelUser, labelPass)
	if user == "" {
		return nil, fmt.Errorf(
			"registry returned 401 unauthorized and no docker.io credentials are configured",
		)
	}

	// ── Step 3: Try authenticated token ──────────────────────────────────────
	token, err = getDockerHubToken(ref.Name, user, pass, c.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated docker hub token: %v", err)
	}

	info, err := c.fetchManifestInfo(manifestURL, "Bearer "+token)
	if err != nil {
		return nil, fmt.Errorf("authenticated docker hub fetch failed: %v", err)
	}

	info.Tag = ref.Tag
	logger.Log.Debugf("Docker Hub authenticated access succeeded for %s digest=%s",
		ref.Name, shortDigest(info.Digest))
	return info, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Private Registry (ghcr.io, ECR, custom, etc.)
// ─────────────────────────────────────────────────────────────────────────────

// getPrivateInfo handles private registries.
// Flow:
//  1. Try anonymous access
//  2. If 401 → resolve credentials
//  3. Try Basic auth
//  4. Try Bearer token via WWW-Authenticate challenge
func (c *Client) getPrivateInfo(ref ImageRef, labelUser, labelPass string) (*RemoteImageInfo, error) {
	scheme := "https"
	if ref.Registry == "localhost" || strings.HasPrefix(ref.Registry, "127.") {
		scheme = "http"
	}

	manifestURL := fmt.Sprintf(
		"%s://%s/v2/%s/manifests/%s",
		scheme, ref.Registry, ref.Name, ref.Tag,
	)

	// ── Step 1: Try anonymous ────────────────────────────────────────────────
	info, err := c.fetchManifestInfo(manifestURL, "")
	if err == nil {
		info.Tag = ref.Tag
		logger.Log.Debugf("Anonymous access succeeded for %s digest=%s",
			ref.Registry, shortDigest(info.Digest))
		return info, nil
	}

	logger.Log.Debugf("Anonymous fetch failed for %s: %v — trying authenticated", ref.Registry, err)

	// ── Step 2: Resolve credentials ──────────────────────────────────────────
	user, pass := resolveCredentials(ref.Registry, labelUser, labelPass)
	if user == "" {
		return nil, fmt.Errorf("private registry auth not configured for %s", ref.Registry)
	}

	creds := &Credentials{Username: user, Password: pass}

	// ── Step 3: Try Basic auth ───────────────────────────────────────────────
	basicHeader := buildBasicAuthHeader(user, pass)
	info, err = c.fetchManifestInfo(manifestURL, basicHeader)
	if err == nil {
		info.Tag = ref.Tag
		logger.Log.Debugf("Basic auth succeeded for %s digest=%s",
			ref.Registry, shortDigest(info.Digest))
		return info, nil
	}

	// ── Step 4: Try Bearer token (ghcr.io, etc.) ─────────────────────────────
	token, tokenErr := c.getBearerToken(ref, scheme, creds)
	if tokenErr != nil {
		logger.Log.Debugf("Bearer token fetch failed for %s: %v", ref.Registry, tokenErr)
		return nil, fmt.Errorf("authentication failed for %s: %v", ref.Registry, err)
	}

	info, err = c.fetchManifestInfo(manifestURL, "Bearer "+token)
	if err != nil {
		return nil, fmt.Errorf("authenticated fetch failed for %s: %v", ref.Registry, err)
	}

	info.Tag = ref.Tag
	logger.Log.Debugf("Bearer auth succeeded for %s digest=%s",
		ref.Registry, shortDigest(info.Digest))
	return info, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Credential Resolution
// ─────────────────────────────────────────────────────────────────────────────

// resolveCredentials returns (username, password) using priority order:
//  1. Label-level credentials (per-container override)
//  2. Per-registry env vars  (SENTINEL_REGISTRY_USER_GHCR_IO)
//  3. Generic env vars       (REPO_USER / REPO_PASS)
//  4. Docker config.json
func resolveCredentials(reg, labelUser, labelPass string) (string, string) {
	// 1. Label-level override (highest priority)
	if labelUser != "" && labelPass != "" {
		logger.Log.Debugf("Using label credentials for %s", reg)
		return labelUser, labelPass
	}

	// 2 - 4. Fall through to GetCredentials
	creds, err := GetCredentials(reg)
	if err != nil {
		logger.Log.Debugf("GetCredentials error for %s: %v", reg, err)
		return "", ""
	}
	if creds == nil {
		return "", ""
	}

	return creds.Username, creds.Password
}

// ─────────────────────────────────────────────────────────────────────────────
// Bearer Token
// ─────────────────────────────────────────────────────────────────────────────

func (c *Client) getBearerToken(ref ImageRef, scheme string, creds *Credentials) (string, error) {
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

	wwwAuth := resp.Header.Get("WWW-Authenticate")
	realm, service, scope := parseBearerChallenge(wwwAuth, ref)
	if realm == "" {
		return "", fmt.Errorf("no Bearer realm in WWW-Authenticate: %s", wwwAuth)
	}

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
func parseBearerChallenge(header string, ref ImageRef) (realm, service, scope string) {
	header = strings.TrimPrefix(header, "Bearer ")
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

// ─────────────────────────────────────────────────────────────────────────────
// HTTP helpers
// ─────────────────────────────────────────────────────────────────────────────

// fetchManifestInfo performs a HEAD request and returns full manifest info.
// Returns digest from Docker-Content-Digest header and media type.
func (c *Client) fetchManifestInfo(manifestURL string, authHeader string) (*RemoteImageInfo, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodHead,
		manifestURL,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	// Accept all manifest types to get correct digest
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
	}, ", "))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close registry response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("registry returned 401 unauthorized")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status: %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return nil, fmt.Errorf("no digest in registry response")
	}

	return &RemoteImageInfo{
		Digest:    digest,
		MediaType: resp.Header.Get("Content-Type"),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Docker Hub token
// ─────────────────────────────────────────────────────────────────────────────

// getDockerHubToken fetches a Docker Hub token.
// Pass empty user/pass for anonymous (public) access.
func getDockerHubToken(imageName, user, pass string, client *http.Client) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s?service=registry.docker.io&scope=repository:%s:pull",
		dockerHubTokenURL, imageName,
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}

	if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
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

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

// HasUpdate checks if a newer image is available by comparing SHA digests.
func (c *Client) HasUpdate(localDigest string, image string) (bool, string, error) {
	return c.HasUpdateWithCreds(localDigest, image, "", "")
}

// HasUpdateWithCreds checks for update with optional label-level credential override.
// Compares local digest vs remote digest (SHA-based — most reliable).
func (c *Client) HasUpdateWithCreds(localDigest, image, labelUser, labelPass string) (bool, string, error) {
	info, err := c.GetRemoteImageInfo(image, labelUser, labelPass)
	if err != nil {
		return false, "", err
	}

	remoteDigest := info.Digest

	logger.Log.Debugf("Digest comparison for %s: local=%s remote=%s",
		image,
		shortDigest(localDigest),
		shortDigest(remoteDigest),
	)

	// ── Compare digests (SHA-based) ───────────────────────────────────────────
	// Normalize both digests before comparing
	// Local digest may be stored as "sha256:abc..." directly
	// Remote digest comes as "sha256:abc..." from Docker-Content-Digest header
	if digestsMatch(localDigest, remoteDigest) {
		logger.Log.Debugf("✔   Digests match — no update for %s", image)
		return false, remoteDigest, nil
	}

	logger.Log.WithFields(logger.Fields{
		"image":         image,
		"tag":           info.Tag,
		"local_digest":  shortDigest(localDigest),
		"remote_digest": shortDigest(remoteDigest),
		"media_type":    info.MediaType,
	}).Info("🆕  Update available (digest mismatch)")

	return true, remoteDigest, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Digest helpers
// ─────────────────────────────────────────────────────────────────────────────

// digestsMatch compares two digest strings.
// Handles cases where one might be prefixed with "sha256:" and the other not.
// Also handles manifest list vs single-platform digest differences.
func digestsMatch(local, remote string) bool {
	if local == "" || remote == "" {
		return false
	}

	// Direct match
	if local == remote {
		return true
	}

	// Normalize: strip algorithm prefix for comparison
	localHash := stripAlgorithmPrefix(local)
	remoteHash := stripAlgorithmPrefix(remote)

	if localHash == remoteHash {
		return true
	}

	// Check if local digest is contained within remote or vice versa
	// This handles cases where local stores a platform-specific digest
	// but remote returns a manifest list digest
	if strings.HasPrefix(localHash, remoteHash) || strings.HasPrefix(remoteHash, localHash) {
		return true
	}

	return false
}

// stripAlgorithmPrefix removes the "sha256:" or "sha512:" prefix from a digest
func stripAlgorithmPrefix(digest string) string {
	if idx := strings.Index(digest, ":"); idx != -1 {
		return digest[idx+1:]
	}
	return digest
}

// shortDigest returns first 12 chars of a digest for logging
func shortDigest(digest string) string {
	hash := stripAlgorithmPrefix(digest)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}