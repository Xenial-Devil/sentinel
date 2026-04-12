package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sentinel/logger"
	"time"
)

const (
	SentinelVersion = "1.0.0"
	SignatureHeader = "X-Sentinel-Signature"
)

// Client sends outbound webhooks
type Client struct {
	URL        string
	Secret     string
	HTTPClient *http.Client
}

// New creates a new webhook client
func New(url string, secret string) *Client {
	return &Client{
		URL:    url,
		Secret: secret,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a webhook event
func (c *Client) Send(event EventType, containerName string, image string) error {
	payload := Payload{
		Event:         event,
		Timestamp:     time.Now(),
		ContainerName: containerName,
		Image:         image,
		Meta: PayloadMeta{
			Host:    getHostname(),
			Version: SentinelVersion,
		},
	}

	return c.sendPayload(payload)
}

// SendWithImages sends webhook with old and new image info
func (c *Client) SendWithImages(
	event EventType,
	containerName string,
	oldImage string,
	newImage string,
) error {
	payload := Payload{
		Event:         event,
		Timestamp:     time.Now(),
		ContainerName: containerName,
		OldImage:      oldImage,
		NewImage:      newImage,
		Meta: PayloadMeta{
			Host:    getHostname(),
			Version: SentinelVersion,
		},
	}

	return c.sendPayload(payload)
}

// SendWithError sends webhook with error info
func (c *Client) SendWithError(
	event EventType,
	containerName string,
	image string,
	err error,
) error {
	payload := Payload{
		Event:         event,
		Timestamp:     time.Now(),
		ContainerName: containerName,
		Image:         image,
		Error:         err.Error(),
		Meta: PayloadMeta{
			Host:    getHostname(),
			Version: SentinelVersion,
		},
	}

	return c.sendPayload(payload)
}

// sendPayload marshals and sends the payload
func (c *Client) sendPayload(payload Payload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		c.URL,
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sentinel/"+SentinelVersion)

	if c.Secret != "" {
		signature := c.signPayload(data)
		req.Header.Set(SignatureHeader, signature)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close webhook response body: %v", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook returned status: %d", resp.StatusCode)
	}

	logger.Log.Debugf("Webhook sent: %s -> %s", payload.Event, c.URL)
	return nil
}

// signPayload creates HMAC SHA256 signature
func (c *Client) signPayload(data []byte) string {
	mac := hmac.New(sha256.New, []byte(c.Secret))
	mac.Write(data)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// getHostname returns the current hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}