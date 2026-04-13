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

var (
	Version   = "dev"
	CommitSHA = "none"
	BuildDate = "unknown"
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.LogLevel, cfg.LogFormat)
	printBanner()

	logger.Log.WithFields(logger.Fields{
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"version": Version,
		"commit":  CommitSHA,
		"built":   BuildDate,
	}).Info("🛡   Sentinel starting up")

	logger.Log.WithFields(logger.Fields{
		"host":            cfg.DockerHost,
		"poll_interval":   cfg.PollInterval,
		"monitor_only":    cfg.MonitorOnly,
		"watch_all":       cfg.WatchAll,
		"label_enable":    cfg.LabelEnable,
		"approval":        cfg.ApprovalEnabled,
		"rollback":        cfg.EnableRollback,
		"rolling_restart": cfg.RollingRestart,
		"scope":           cfg.Scope,
		"no_pull":         cfg.NoPull,
		"no_restart":      cfg.NoRestart,
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

	// Metrics
	var m *metrics.Metrics
	if cfg.MetricsEnabled {
		m = metrics.New()
		m.StartServer(cfg.MetricsPort)
		logger.LogMetricsStart(cfg.MetricsPort)
	}

	// Watcher
	w := watcher.New(client, cfg, m)

	// API
	if cfg.APIEnabled {
		api.InitApproval(cfg)
		a := api.New(cfg, w, client) // pass docker client
		a.Start()
		defer a.Stop()
		logger.LogAPIStart(cfg.APIPort, cfg.APIToken != "")
	}

	go handleShutdown(client)
	w.Start()
}

func printBanner() {
	logger.Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Log.Info("   ███████╗███████╗███╗   ██╗████████╗██╗███╗   ██╗███████╗  ")
	logger.Log.Info("   ██╔════╝██╔════╝████╗  ██║╚══██╔══╝██║████╗  ██║██╔════╝  ")
	logger.Log.Info("   ███████╗█████╗  ██╔██╗ ██║   ██║   ██║██╔██╗ ██║█████╗    ")
	logger.Log.Info("   ╚════██║██╔══╝  ██║╚██╗██║   ██║   ██║██║╚██╗██║██╔══╝    ")
	logger.Log.Info("   ███████║███████╗██║ ╚████║   ██║   ██║██║ ╚████║███████╗  ")
	logger.Log.Info("   ╚══════╝╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═╝╚═╝  ╚═══╝╚══════╝  ")
	logger.Log.Infof("   🛡   Sentinel  v%s  (%s)  built %s", Version, CommitSHA, BuildDate)
	logger.Log.Infof("   🖥   Runtime   Go %s  %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	logger.Log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func handleShutdown(client *docker.Client) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Log.WithField("signal", sig.String()).Warn("⚠   Shutdown signal received")
	logger.LogShutdown(sig.String())
	client.Close()
	logger.Log.Info("👋  Sentinel stopped. Goodbye.")
	os.Exit(0)
}