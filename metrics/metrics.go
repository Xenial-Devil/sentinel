package metrics

import (
	"fmt"
	"net/http"
	"sentinel/logger"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all prometheus metrics
type Metrics struct {
	UpdatesTotal      prometheus.Counter
	UpdatesFailed     prometheus.Counter
	RollbacksTotal    prometheus.Counter
	PullsTotal        prometheus.Counter
	PullsFailed       prometheus.Counter
	ContainersWatched prometheus.Gauge
	UpdatesPending    prometheus.Gauge
	UpdateDuration    prometheus.Histogram
	PullDuration      prometheus.Histogram
	registry          *prometheus.Registry
}

// New creates and registers all metrics
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		UpdatesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sentinel_updates_total",
			Help: "Total number of successful container updates",
		}),
		UpdatesFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sentinel_updates_failed_total",
			Help: "Total number of failed container updates",
		}),
		RollbacksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sentinel_rollbacks_total",
			Help: "Total number of container rollbacks",
		}),
		PullsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sentinel_pulls_total",
			Help: "Total number of image pulls",
		}),
		PullsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sentinel_pulls_failed_total",
			Help: "Total number of failed image pulls",
		}),
		ContainersWatched: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sentinel_containers_watched",
			Help: "Number of containers currently being watched",
		}),
		UpdatesPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sentinel_updates_pending",
			Help: "Number of updates pending approval",
		}),
		UpdateDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "sentinel_update_duration_seconds",
			Help:    "Time taken to fully update a container",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
		}),
		PullDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "sentinel_pull_duration_seconds",
			Help:    "Time taken to pull an image",
			Buckets: []float64{1, 5, 10, 30, 60, 120},
		}),
	}

	reg.MustRegister(
		m.UpdatesTotal,
		m.UpdatesFailed,
		m.RollbacksTotal,
		m.PullsTotal,
		m.PullsFailed,
		m.ContainersWatched,
		m.UpdatesPending,
		m.UpdateDuration,
		m.PullDuration,
	)

	return m
}

// StartServer starts the metrics HTTP server
func (m *Metrics) StartServer(port int) {
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, "ok"); err != nil {
			logger.Log.Warnf("Failed to write health response: %v", err)
		}
	})

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Log.Errorf("Metrics server error: %v", err)
		}
	}()

	logger.Log.Infof("📊  Metrics available at http://localhost%s/metrics", addr)
}

// ── Counters ──────────────────────────────────────────────────────────────────

func (m *Metrics) RecordUpdate() {
	m.UpdatesTotal.Inc()
}

func (m *Metrics) RecordUpdateFailed() {
	m.UpdatesFailed.Inc()
}

func (m *Metrics) RecordRollback() {
	m.RollbacksTotal.Inc()
}

func (m *Metrics) RecordPull() {
	m.PullsTotal.Inc()
}

func (m *Metrics) RecordPullFailed() {
	m.PullsFailed.Inc()
}

// ── Gauges ────────────────────────────────────────────────────────────────────

func (m *Metrics) SetContainersWatched(count int) {
	m.ContainersWatched.Set(float64(count))
}

func (m *Metrics) SetUpdatesPending(count int) {
	m.UpdatesPending.Set(float64(count))
}

// ── Histograms ────────────────────────────────────────────────────────────────

// ObserveUpdateDuration records total update duration
func (m *Metrics) ObserveUpdateDuration(d time.Duration) {
	m.UpdateDuration.Observe(d.Seconds())
}

// ObservePullDuration records image pull duration
func (m *Metrics) ObservePullDuration(d time.Duration) {
	m.PullDuration.Observe(d.Seconds())
}