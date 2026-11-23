package health

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/pkg/models"
)

func TestHealthMonitorSSRFBlocked(t *testing.T) {
	m := metrics.NewMetrics()
	mon := NewMonitor(m)

	// Upstream pointing to cloud metadata endpoint without permission
	upstream := models.Upstream{
		ID:                  "up-test-ssrf",
		Slug:                "metadata-target",
		TargetURL:           "http://169.254.169.254",
		HealthCheckPath:     "/latest/meta-data/",
		AllowLocalhost:      false,
		AllowPrivateNetwork: false,
	}

	mon.probeUpstream(upstream)

	status, ok := mon.GetStatus("up-test-ssrf")
	if !ok {
		t.Fatal("expected status for probe to be recorded")
	}

	if status.Healthy {
		t.Fatal("expected probe to metadata IP to be marked unhealthy")
	}

	if status.LastErrorMessage == "" {
		t.Fatal("expected error message explaining connection block")
	}
}

func TestHealthMonitorTransitions(t *testing.T) {
	statusCode := http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = fmt.Fprintln(w, "OK")
	}))
	defer ts.Close()

	m := metrics.NewMetrics()
	mon := NewMonitor(m)

	upstream := models.Upstream{
		ID:                  "up-test-server",
		Slug:                "test-server",
		TargetURL:           ts.URL,
		HealthCheckPath:     "/healthz",
		AllowLocalhost:      true,
		AllowPrivateNetwork: true,
	}

	// 1. Initial successful probe -> marked healthy
	mon.probeUpstream(upstream)
	st, ok := mon.GetStatus("up-test-server")
	if !ok || !st.Healthy {
		t.Fatalf("expected upstream to be healthy after 200 OK probe, got healthy=%v", st.Healthy)
	}

	// 2. Failure 1 -> still healthy (tolerance threshold is 3)
	statusCode = http.StatusInternalServerError
	mon.probeUpstream(upstream)
	st, _ = mon.GetStatus("up-test-server")
	if !st.Healthy || st.ConsecutiveFailures != 1 {
		t.Fatalf("expected healthy=true with consecutive failures=1, got healthy=%v, failures=%d", st.Healthy, st.ConsecutiveFailures)
	}

	// 3. Failure 2 -> still healthy
	mon.probeUpstream(upstream)
	st, _ = mon.GetStatus("up-test-server")
	if !st.Healthy || st.ConsecutiveFailures != 2 {
		t.Fatalf("expected healthy=true with consecutive failures=2, got healthy=%v, failures=%d", st.Healthy, st.ConsecutiveFailures)
	}

	// 4. Failure 3 -> marked unhealthy
	mon.probeUpstream(upstream)
	st, _ = mon.GetStatus("up-test-server")
	if st.Healthy || st.ConsecutiveFailures != 3 {
		t.Fatalf("expected healthy=false with consecutive failures=3, got healthy=%v, failures=%d", st.Healthy, st.ConsecutiveFailures)
	}

	// 5. Recovery -> single 200 OK recovers health
	statusCode = http.StatusOK
	mon.probeUpstream(upstream)
	st, _ = mon.GetStatus("up-test-server")
	if !st.Healthy || st.ConsecutiveFailures != 0 {
		t.Fatalf("expected healthy=true with consecutive failures=0 on recovery, got healthy=%v, failures=%d", st.Healthy, st.ConsecutiveFailures)
	}
}
