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

// Build-time variables - set via ldflags
var (
	Version   = "dev"
	CommitSHA = "none"
	BuildDate = "unknown"
)

func main() {
	// Load config
	cfg := config.Load()

	// Setup logger
	logger.Init(cfg.LogLevel, cfg.LogFormat)

	// Print banner
	printBanner()

	// Startup info
	logger.Log.WithFields(logger.Fields{
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"version": Version,
		"commit":  CommitSHA,
		"built":   BuildDate,
	}).Info("🛡   Sentinel starting up")

	logger.Log.WithFields(logger.Fields{
		"host":          cfg.DockerHost,
		"poll_interval": cfg.PollInterval,
		"monitor_only":  cfg.MonitorOnly,
		"watch_all":     cfg.WatchAll,
		"label_enable":  cfg.LabelEnable,
	}).Info("⚙   Configuration loaded")

	if cfg.LabelEnable {
		logger.Log.WithFields(logger.Fields{
			"label": cfg.WatchLabel,
			"value": cfg.WatchLabelValue,
		}).Info("🏷   Watch label configured")
	}

	// Connect to Docker
	client, err := docker.New(cfg)
	if err != nil {
		logger.Log.WithError(err).Fatal("❌  Failed to connect to Docker daemon")
	}
	defer client.Close()

	// Start metrics server
	if cfg.MetricsEnabled {
		m := metrics.New()
		m.StartServer(cfg.MetricsPort)
		logger.LogMetricsStart(cfg.MetricsPort)
	}

	// Create watcher
	w := watcher.New(client, cfg)

	// Start API server
	if cfg.APIEnabled {
		api.InitApproval(cfg)
		a := api.New(cfg, w)
		a.Start()
		defer a.Stop()
		logger.LogAPIStart(cfg.APIPort, cfg.APIToken != "")
	}

	// Handle shutdown signals
	go handleShutdown(client)

	// Start watcher - blocks forever
	w.Start()
}

// printBanner prints the sentinel startup banner
func printBanner() {
	logger.Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Log.Info("                                                              ")
	logger.Log.Info("   ███████╗███████╗███╗   ██╗████████╗██╗███╗   ██╗███████╗  ")
	logger.Log.Info("   ██╔════╝██╔════╝████╗  ██║╚══██╔══╝██║████╗  ██║██╔════╝  ")
	logger.Log.Info("   ███████╗█████╗  ██╔██╗ ██║   ██║   ██║██╔██╗ ██║█████╗    ")
	logger.Log.Info("   ╚════██║██╔══╝  ██║╚██╗██║   ██║   ██║██║╚██╗██║██╔══╝    ")
	logger.Log.Info("   ███████║███████╗██║ ╚████║   ██║   ██║██║ ╚████║███████╗  ")
	logger.Log.Info("   ╚══════╝╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═╝╚═╝  ╚═══╝╚══════╝  ")
	logger.Log.Info("                                                              ")
	logger.Log.Infof("   🛡   Safe Docker Container Auto-Updater                    ")
	logger.Log.Infof("   📦  Version    : %s                                       ", Version)
	logger.Log.Infof("   🔨  Commit     : %s                                       ", CommitSHA)
	logger.Log.Infof("   📅  Build Date : %s                                       ", BuildDate)
	logger.Log.Infof("   🖥   Runtime    : Go %s %s/%s                             ", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	logger.Log.Info("                                                              ")
	logger.Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// handleShutdown listens for OS signals and gracefully shuts down
func handleShutdown(client *docker.Client) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-sigChan

	logger.Log.WithField("signal", sig.String()).
		Warn("⚠   Shutdown signal received")

	logger.LogShutdown(sig.String())

	client.Close()

	logger.Log.Info("👋  Sentinel stopped. Goodbye.")
	os.Exit(0)
}