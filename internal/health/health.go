package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/pkg/models"
)

// Monitor runs active and passive upstream health probes.
type Monitor struct {
	mu       sync.RWMutex
	statuses map[string]*models.UpstreamHealth // key: upstream_id
	client   *http.Client
	metrics  *metrics.Metrics
	stopCh   chan struct{}
}

// NewMonitor creates a health monitor.
func NewMonitor(m *metrics.Metrics) *Monitor {
	return &Monitor{
		statuses: make(map[string]*models.UpstreamHealth),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		metrics: m,
		stopCh:  make(chan struct{}),
	}
}

// Start begins the active polling loop for registered upstreams.
func (mon *Monitor) Start(getUpstreams func() []models.Upstream, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-mon.stopCh:
				ticker.Stop()
				return
			case <-ticker.C:
				upstreams := getUpstreams()
				for _, u := range upstreams {
					if u.IsActive {
						go mon.probeUpstream(u)
					}
				}
			}
		}
	}()
}

// Stop shuts down the background monitor loop.
func (mon *Monitor) Stop() {
	close(mon.stopCh)
}

func (mon *Monitor) probeUpstream(u models.Upstream) {
	probePath := u.HealthCheckPath
	if probePath == "" {
		probePath = "/healthz"
	}
	probeURL := fmt.Sprintf("%s%s", u.TargetURL, probePath)

	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), "GET", probeURL, nil)
	if err != nil {
		mon.recordResult(u.ID, u.Slug, false, 0, 0, err.Error())
		return
	}
	req.Header.Set("User-Agent", "FaultLine-HealthProbe/1.0")

	resp, err := mon.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		mon.recordResult(u.ID, u.Slug, false, 0, latency, err.Error())
		return
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	var errMsg string
	if !healthy {
		errMsg = fmt.Sprintf("probe returned status %d", resp.StatusCode)
	}

	mon.recordResult(u.ID, u.Slug, healthy, resp.StatusCode, latency, errMsg)
}

func (mon *Monitor) recordResult(upstreamID, slug string, healthy bool, statusCode int, latency time.Duration, errMsg string) {
	mon.mu.Lock()
	defer mon.mu.Unlock()

	st, ok := mon.statuses[upstreamID]
	if !ok {
		st = &models.UpstreamHealth{
			UpstreamID: upstreamID,
			Healthy:    true,
		}
		mon.statuses[upstreamID] = st
	}

	st.LastCheckTime = time.Now().UTC()
	st.LatencyMs = latency.Milliseconds()
	st.LastHTTPStatus = statusCode
	st.LastErrorMessage = errMsg

	if healthy {
		st.Healthy = true
		st.ConsecutiveFailures = 0
		if mon.metrics != nil {
			mon.metrics.UpstreamHealthStatus.WithLabelValues(slug).Set(1)
			mon.metrics.UpstreamLatencySec.WithLabelValues(slug).Set(latency.Seconds())
		}
	} else {
		st.ConsecutiveFailures++
		if st.ConsecutiveFailures >= 3 {
			st.Healthy = false
		}
		if mon.metrics != nil {
			mon.metrics.UpstreamHealthStatus.WithLabelValues(slug).Set(0)
			mon.metrics.UpstreamLatencySec.WithLabelValues(slug).Set(latency.Seconds())
		}
	}
}

// GetStatus retrieves health telemetry for an upstream.
func (mon *Monitor) GetStatus(upstreamID string) (*models.UpstreamHealth, bool) {
	mon.mu.RLock()
	defer mon.mu.RUnlock()
	st, ok := mon.statuses[upstreamID]
	if !ok {
		return nil, false
	}
	copied := *st
	return &copied, true
}

// ListStatuses returns health telemetry for all tracked upstreams.
func (mon *Monitor) ListStatuses() []models.UpstreamHealth {
	mon.mu.RLock()
	defer mon.mu.RUnlock()
	results := make([]models.UpstreamHealth, 0, len(mon.statuses))
	for _, v := range mon.statuses {
		results = append(results, *v)
	}
	return results
}
