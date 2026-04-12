package scheduler

import (
	"sentinel/config"
	"sentinel/logger"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler handles timing of update cycles
type Scheduler struct {
	Config   *config.Config
	cron     *cron.Cron
	ticker   *time.Ticker
	runFunc  func()
}

// New creates a new Scheduler
func New(cfg *config.Config, runFunc func()) *Scheduler {
	return &Scheduler{
		Config:  cfg,
		runFunc: runFunc,
	}
}

// Start begins scheduling based on config
func (s *Scheduler) Start() {
	// Use cron if configured
	if s.Config.CronSchedule != "" {
		s.startCron()
		return
	}

	// Otherwise use interval polling
	s.startInterval()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	// Stop cron if running
	if s.cron != nil {
		s.cron.Stop()
		logger.Log.Info("Cron scheduler stopped")
	}

	// Stop ticker if running
	if s.ticker != nil {
		s.ticker.Stop()
		logger.Log.Info("Interval scheduler stopped")
	}
}

// startCron starts cron based scheduling
func (s *Scheduler) startCron() {
	logger.Log.Infof("Starting cron scheduler: %s", s.Config.CronSchedule)

	s.cron = cron.New()

	// Add job to cron
	_, err := s.cron.AddFunc(s.Config.CronSchedule, func() {
		logger.Log.Info("Cron triggered check cycle")
		s.runFunc()
	})

	if err != nil {
		logger.Log.Errorf("Invalid cron schedule: %v", err)
		logger.Log.Info("Falling back to interval polling")
		s.startInterval()
		return
	}

	// Run once immediately
	logger.Log.Info("Running initial check cycle")
	s.runFunc()

	// Start cron
	s.cron.Start()

	// Block forever
	select {}
}

// startInterval starts interval based polling
func (s *Scheduler) startInterval() {
	interval := time.Duration(s.Config.PollInterval) * time.Second
	logger.Log.Infof("Starting interval scheduler: every %d seconds",
		s.Config.PollInterval)

	s.ticker = time.NewTicker(interval)

	// Run once immediately
	logger.Log.Info("Running initial check cycle")
	s.runFunc()

	// Run on interval
	for range s.ticker.C {
		logger.Log.Info("Interval triggered check cycle")
		s.runFunc()
	}
}