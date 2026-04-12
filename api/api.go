package api

import (
	"context"
	"fmt"
	"net/http"
	"sentinel/config"
	"sentinel/logger"
	"time"
)

// API is the HTTP API server
type API struct {
	Config  *config.Config
	Server  *http.Server
	Watcher WatcherInterface
}

// WatcherInterface defines what watcher methods API can call
type WatcherInterface interface {
	RunCycle()
}

// New creates a new API server
func New(cfg *config.Config, watcher WatcherInterface) *API {
	a := &API{
		Config:  cfg,
		Watcher: watcher,
	}

	// Setup server
	a.Server = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.APIPort),
		Handler:      a.setupRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	return a
}

// Start starts the API server
func (a *API) Start() {
	logger.Log.Infof("API server starting on :%d", a.Config.APIPort)

	go func() {
		if err := a.Server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				logger.Log.Errorf("API server error: %v", err)
			}
		}
	}()

	logger.Log.Infof("API available at http://localhost:%d", a.Config.APIPort)
}

// Stop gracefully stops the API server
func (a *API) Stop() {
	logger.Log.Info("Stopping API server...")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := a.Server.Shutdown(ctx); err != nil {
		logger.Log.Errorf("API server shutdown error: %v", err)
	}

	logger.Log.Info("API server stopped")
}