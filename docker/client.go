package docker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sentinel/config"
	"sentinel/logger"
	"time"

	dockerclient "github.com/docker/docker/client"
)

// Client wraps the Docker client
type Client struct {
	CLI *dockerclient.Client
}

// New creates a new Docker client with full TLS support
func New(cfg *config.Config) (*Client, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
	}

	// Apply host override
	if cfg.DockerHost != "" {
		opts = append(opts, dockerclient.WithHost(cfg.DockerHost))
	} else {
		opts = append(opts, dockerclient.FromEnv)
	}

	// Apply TLS if configured
	if cfg.DockerTLSVerify && cfg.DockerCertPath != "" {
		tlsConfig, err := buildTLSConfig(cfg.DockerCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %v", err)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
			Timeout: 60 * time.Second,
		}

		opts = append(opts, dockerclient.WithHTTPClient(httpClient))

		logger.Log.WithField("cert_path", cfg.DockerCertPath).
			Info("🔒  Docker TLS enabled")
	}

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	logger.Log.WithField("host", cfg.DockerHost).
		Info("🐳  Connected to Docker daemon")

	return &Client{CLI: cli}, nil
}

// Close closes the Docker client
func (c *Client) Close() {
	if err := c.CLI.Close(); err != nil {
		logger.Log.Warnf("Docker client close error: %v", err)
	}
	logger.Log.Info("Docker client closed")
}

// buildTLSConfig builds a TLS config from cert path
// Expects: ca.pem, cert.pem, key.pem in the cert path directory
func buildTLSConfig(certPath string) (*tls.Config, error) {
	caFile   := filepath.Join(certPath, "ca.pem")
	certFile := filepath.Join(certPath, "cert.pem")
	keyFile  := filepath.Join(certPath, "key.pem")

	// Load client cert
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert/key: %v", err)
	}

	// Load CA cert
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %v", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}