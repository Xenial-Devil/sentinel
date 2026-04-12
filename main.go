package main

import (
	"os"
	"os/signal"
	"runtime"
	"sentinel/api"
	"sentinel/config"
	"sentinel/docker"
	"sentinel/logger"
	"sentinel/metrics"
	"sentinel/watcher"
	"syscall"
)

func main() {
	// Load config
	cfg := config.Load()

	// Setup logger
	logger.Init(cfg.LogLevel, cfg.LogFormat)

	// Print banner
	printBanner()

	logger.Log.Infof("OS Detected:  %s", runtime.GOOS)
	logger.Log.Infof("Docker Host:  %s", cfg.DockerHost)
	logger.Log.Infof("Poll Interval: %d seconds", cfg.PollInterval)
	logger.Log.Infof("Monitor Only: %v", cfg.MonitorOnly)
	logger.Log.Infof("Watch All:    %v", cfg.WatchAll)

	// Connect to Docker
	client, err := docker.New(cfg)
	if err != nil {
		logger.Log.Fatalf("Failed to connect to Docker: %v", err)
	}
	defer client.Close()

	// Start metrics server
	if cfg.MetricsEnabled {
		m := metrics.New()
		m.StartServer(cfg.MetricsPort)
	}

	// Create watcher
	w := watcher.New(client, cfg)

	// Start API server
	if cfg.APIEnabled {
		api.InitApproval(cfg)
		a := api.New(cfg, w)
		a.Start()
		defer a.Stop()
	}

	// Handle shutdown signals
	go handleShutdown(client)

	// Start watcher - blocks forever
	w.Start()
}

// printBanner prints the sentinel banner
func printBanner() {
	banner := `
 ____  ___ _   _ _____ ___ _   _ _____ _     
/ ___|| __| \ | |_   _|_ _| \ | | ____| |    
\___ \|  _||  \| | | |  | ||  \| |  _| | |    
 ___) | |__| |\  | | |  | || |\  | |___| |___ 
|____/|___|_| \_| |_| |___|_| \_|_____|_____|

🛡️  Sentinel - Safe Container Deployment Controller
     Version: 1.0.0
`
	logger.Log.Info(banner)
}

// handleShutdown handles OS shutdown signals
func handleShutdown(client *docker.Client) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-sigChan
	logger.Log.Infof("Received signal: %v - shutting down...", sig)

	// Close docker client
	client.Close()

	logger.Log.Info("Sentinel stopped")
	os.Exit(0)
}