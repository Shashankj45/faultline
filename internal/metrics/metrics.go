package metrics

import (
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds Prometheus collectors and in-memory live telemetry.
type Metrics struct {
	reg *prometheus.Registry

	HTTPRequestsTotal       *prometheus.CounterVec
	HTTPRequestDurationSec  *prometheus.HistogramVec
	InjectedFaultsTotal     *prometheus.CounterVec
	KillSwitchActive        prometheus.Gauge
	ActiveExperiments       *prometheus.GaugeVec
	UpstreamHealthStatus    *prometheus.GaugeVec
	UpstreamLatencySec      *prometheus.GaugeVec

	// Live sliding-window stats for real-time Web UI telemetry
	mu             sync.RWMutex
	recentRequests []requestRecord
	recentFaults   map[string]int64 // key: fault_type -> count
}

type requestRecord struct {
	Timestamp     time.Time
	UpstreamSlug  string
	StatusCode    int
	Duration      time.Duration
	InjectedFault string
}

// LiveStats provides summarized real-time telemetry for the Admin Dashboard.
type LiveStats struct {
	RequestsPerSec   float64          `json:"requests_per_sec"`
	ErrorRatePercent float64          `json:"error_rate_percent"`
	P50LatencyMs     float64          `json:"p50_latency_ms"`
	P95LatencyMs     float64          `json:"p95_latency_ms"`
	P99LatencyMs     float64          `json:"p99_latency_ms"`
	TotalRequests    int64            `json:"total_requests"`
	TotalFaults      int64            `json:"total_faults"`
	FaultBreakdown   map[string]int64 `json:"fault_breakdown"`
	RecentHistory    []DataPoint      `json:"recent_history"`
}

type DataPoint struct {
	TimeIso       string  `json:"time_iso"`
	RequestCount  int     `json:"request_count"`
	InjectedCount int     `json:"injected_count"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// NewMetrics initializes and registers Prometheus collectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		reg: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "faultline_http_requests_total",
				Help: "Total number of HTTP requests processed by FaultLine proxy",
			},
			[]string{"upstream", "method", "status", "injected_fault"},
		),
		HTTPRequestDurationSec: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "faultline_http_request_duration_seconds",
				Help:    "Histogram of HTTP request durations through FaultLine",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"upstream", "method"},
		),
		InjectedFaultsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "faultline_injected_faults_total",
				Help: "Total count of chaos faults injected by FaultLine",
			},
			[]string{"upstream", "experiment_id", "fault_type"},
		),
		KillSwitchActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "faultline_killswitch_active",
				Help: "Indicates whether the global emergency kill switch is active (1 = active, 0 = inactive)",
			},
		),
		ActiveExperiments: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "faultline_active_experiments",
				Help: "Number of active experiments by status",
			},
			[]string{"status"},
		),
		UpstreamHealthStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "faultline_upstream_health_status",
				Help: "Upstream health status (1 = healthy, 0 = unhealthy)",
			},
			[]string{"upstream"},
		),
		UpstreamLatencySec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "faultline_upstream_latency_seconds",
				Help: "Most recent health probe latency in seconds",
			},
			[]string{"upstream"},
		),
		recentFaults: make(map[string]int64),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDurationSec,
		m.InjectedFaultsTotal,
		m.KillSwitchActive,
		m.ActiveExperiments,
		m.UpstreamHealthStatus,
		m.UpstreamLatencySec,
	)

	return m
}

// Handler returns the HTTP handler exposing Prometheus /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// RecordRequest tracks a completed request in Prometheus and live history.
func (m *Metrics) RecordRequest(upstream, method, status string, duration time.Duration, injectedFault string) {
	m.HTTPRequestsTotal.WithLabelValues(upstream, method, status, injectedFault).Inc()
	m.HTTPRequestDurationSec.WithLabelValues(upstream, method).Observe(duration.Seconds())

	statusCode := 200
	if status != "" {
		for _, c := range status {
			if c >= '0' && c <= '9' {
				statusCode = statusCode*10 + int(c-'0')
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rec := requestRecord{
		Timestamp:     time.Now().UTC(),
		UpstreamSlug:  upstream,
		StatusCode:    statusCode,
		Duration:      duration,
		InjectedFault: injectedFault,
	}

	m.recentRequests = append(m.recentRequests, rec)
	if injectedFault != "" && injectedFault != "none" {
		m.recentFaults[injectedFault]++
	}

	// Prune history older than 5 minutes or > 5000 records
	if len(m.recentRequests) > 5000 {
		m.recentRequests = m.recentRequests[len(m.recentRequests)-5000:]
	}
}

// RecordInjectedFault increments fault counters.
func (m *Metrics) RecordInjectedFault(upstream, expID, faultType string) {
	m.InjectedFaultsTotal.WithLabelValues(upstream, expID, faultType).Inc()
}

// SetKillSwitchStatus updates the kill switch metric.
func (m *Metrics) SetKillSwitchStatus(active bool) {
	if active {
		m.KillSwitchActive.Set(1)
	} else {
		m.KillSwitchActive.Set(0)
	}
}

// GetLiveStats computes real-time stats for the dashboard.
func (m *Metrics) GetLiveStats() LiveStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()
	cutoff := now.Add(-60 * time.Second)

	var (
		count60s       int
		errors60s      int
		faultCount60s  int
		durations      []float64
		faultBreakdown = make(map[string]int64)
	)

	for k, v := range m.recentFaults {
		faultBreakdown[k] = v
	}

	for _, r := range m.recentRequests {
		if r.Timestamp.After(cutoff) {
			count60s++
			durations = append(durations, float64(r.Duration.Milliseconds()))
			if r.StatusCode >= 500 {
				errors60s++
			}
			if r.InjectedFault != "" && r.InjectedFault != "none" {
				faultCount60s++
			}
		}
	}

	rps := float64(count60s) / 60.0
	errRate := 0.0
	if count60s > 0 {
		errRate = (float64(errors60s) / float64(count60s)) * 100.0
	}

	var p50, p95, p99 float64
	if len(durations) > 0 {
		sort.Float64s(durations)
		p50 = durations[int(float64(len(durations))*0.50)]
		p95 = durations[int(float64(len(durations))*0.95)]
		p99 = durations[int(float64(len(durations))*0.99)]
	}

	// Generate 10-second bucket historical timeline
	var history []DataPoint
	bucketSize := 10 * time.Second
	for i := 5; i >= 0; i-- {
		bStart := now.Add(-time.Duration(i+1) * bucketSize)
		bEnd := now.Add(-time.Duration(i) * bucketSize)

		var bCount, bFaults int
		var bDurSum float64

		for _, r := range m.recentRequests {
			if (r.Timestamp.Equal(bStart) || r.Timestamp.After(bStart)) && r.Timestamp.Before(bEnd) {
				bCount++
				bDurSum += float64(r.Duration.Milliseconds())
				if r.InjectedFault != "" && r.InjectedFault != "none" {
					bFaults++
				}
			}
		}

		avgDur := 0.0
		if bCount > 0 {
			avgDur = bDurSum / float64(bCount)
		}

		history = append(history, DataPoint{
			TimeIso:       bEnd.Format("15:04:05"),
			RequestCount:  bCount,
			InjectedCount: bFaults,
			AvgLatencyMs:  avgDur,
		})
	}

	return LiveStats{
		RequestsPerSec:   rps,
		ErrorRatePercent: errRate,
		P50LatencyMs:     p50,
		P95LatencyMs:     p95,
		P99LatencyMs:     p99,
		TotalRequests:    int64(len(m.recentRequests)),
		TotalFaults:      int64(faultCount60s),
		FaultBreakdown:   faultBreakdown,
		RecentHistory:    history,
	}
}
