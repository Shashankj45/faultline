package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/faultline-proxy/faultline/internal/api"
	"github.com/faultline-proxy/faultline/internal/audit"
	"github.com/faultline-proxy/faultline/internal/auth"
	"github.com/faultline-proxy/faultline/internal/config"
	"github.com/faultline-proxy/faultline/internal/faults"
	"github.com/faultline-proxy/faultline/internal/health"
	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/internal/mockservices"
	"github.com/faultline-proxy/faultline/internal/proxy"
	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/internal/telemetry"
	"github.com/faultline-proxy/faultline/pkg/models"
)

func main() {
	log.Println("[INFO] Initializing FaultLine Chaos Reverse Proxy...")
	cfg := config.Load()

	// 1. Initialize Storage Engine
	sqlStore, err := store.NewSQLStore(cfg.Driver, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize storage engine (%s): %v", cfg.Driver, err)
	}
	defer sqlStore.Close()
	log.Printf("[INFO] Database connected successfully via %s", cfg.Driver)

	// 2. Start Embedded Mock Services if enabled
	var mockCluster *mockservices.MockCluster
	if cfg.MockServices {
		mockCluster = mockservices.Start()
		defer mockCluster.Close()
		log.Println("[INFO] Mock microservices started:")
		log.Printf("       -> Order Service:     %s", mockCluster.OrderServer.URL)
		log.Printf("       -> Payment Gateway:   %s", mockCluster.PaymentServer.URL)
		log.Printf("       -> Inventory Service: %s", mockCluster.InventoryServer.URL)

		seedDefaultUpstreams(sqlStore, mockCluster)
	}

	// 3. Core Engine & Observability Components
	_, otelShutdown := telemetry.InitTracer("faultline-proxy")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = otelShutdown(ctx)
	}()

	metricsCollector := metrics.NewMetrics()
	faultEngine := faults.NewEngine()
	auditLogger := audit.NewLogger(sqlStore)
	authService := auth.NewAuthService(sqlStore)
	healthMonitor := health.NewMonitor(metricsCollector)
	reverseProxyServer := proxy.NewReverseProxyServer(sqlStore, faultEngine, metricsCollector)

	// Start Health Monitoring loop
	healthMonitor.Start(func() []models.Upstream {
		upstreams, _ := sqlStore.ListUpstreams(context.Background())
		return upstreams
	}, 10*time.Second)
	defer healthMonitor.Stop()

	// 4. API & Control Plane Router
	apiRouter := api.NewRouter(sqlStore, reverseProxyServer, healthMonitor, metricsCollector, auditLogger, authService)

	// 5. Root Multiplexer
	rootMux := http.NewServeMux()

	// Proxy Traffic Route
	rootMux.HandleFunc("/proxy/", func(w http.ResponseWriter, r *http.Request) {
		reverseProxyServer.ServeHTTP(w, r)
	})

	// Control Plane API & Prometheus Metrics
	rootMux.Handle("/api/", apiRouter)
	rootMux.Handle("/metrics", apiRouter)

	// Static Web UI or Fallback
	webHandler := createWebHandler(cfg.StaticDir)
	rootMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/proxy/") || strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics" {
			return
		}
		// If custom header routing is used, proxy directly
		if slug := r.Header.Get("X-FaultLine-Upstream"); slug != "" {
			reverseProxyServer.ServeHTTP(w, r)
			return
		}
		webHandler(w, r)
	})

	serverAddr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      rootMux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown channel
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] ========================================================")
		log.Printf("[INFO] FaultLine Reverse Proxy listening on http://localhost:%d", cfg.Port)
		log.Printf("[INFO] Web UI Console:        http://localhost:%d/", cfg.Port)
		log.Printf("[INFO] Control Plane API:     http://localhost:%d/api/v1/health", cfg.Port)
		log.Printf("[INFO] Prometheus Metrics:    http://localhost:%d/metrics", cfg.Port)
		log.Printf("[INFO] Proxy Ingress Route:   http://localhost:%d/proxy/{upstream_slug}/...", cfg.Port)
		log.Printf("[INFO] ========================================================")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[FATAL] Server listening error: %v", err)
		}
	}()

	<-stopChan
	log.Println("[INFO] Shutting down FaultLine server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Graceful shutdown failed: %v", err)
	} else {
		log.Println("[INFO] FaultLine server stopped cleanly.")
	}
}

func seedDefaultUpstreams(s *store.SQLStore, cluster *mockservices.MockCluster) {
	ctx := context.Background()

	// Seed Upstreams
	upstreams := []models.Upstream{
		{
			ID:                  "ups_orders",
			Name:                "Order Processing Service",
			Slug:                "orders",
			TargetURL:           cluster.OrderServer.URL,
			AllowLocalhost:      true,
			AllowPrivateNetwork: true,
			TimeoutSeconds:      15,
			MaxIdleConns:        100,
			HealthCheckPath:     "/healthz",
			IsActive:            true,
		},
		{
			ID:                  "ups_payments",
			Name:                "Payment Gateway Service",
			Slug:                "payments",
			TargetURL:           cluster.PaymentServer.URL,
			AllowLocalhost:      true,
			AllowPrivateNetwork: true,
			TimeoutSeconds:      10,
			MaxIdleConns:        50,
			HealthCheckPath:     "/healthz",
			IsActive:            true,
		},
		{
			ID:                  "ups_inventory",
			Name:                "Global Inventory Microservice",
			Slug:                "inventory",
			TargetURL:           cluster.InventoryServer.URL,
			AllowLocalhost:      true,
			AllowPrivateNetwork: true,
			TimeoutSeconds:      20,
			MaxIdleConns:        80,
			HealthCheckPath:     "/healthz",
			IsActive:            true,
		},
	}

	for _, u := range upstreams {
		if err := s.CreateUpstream(ctx, &u); err != nil {
			// Update target URL if mock services restarted with fresh ephemeral ports
			_ = s.UpdateUpstream(ctx, &u)
		}
	}

	// Seed Sample Running Experiment
	exp := &models.Experiment{
		ID:                 "exp_blackfriday",
		UpstreamID:         "ups_payments",
		Name:               "Black Friday Resilience Stress Test",
		Description:        "Simulate p99 checkout latency spike and downstream 503 errors",
		Status:             models.StatusRunning,
		MaxDurationSeconds: 7200,
		CreatedBy:          "lead-sre",
	}
	_ = s.CreateExperiment(ctx, exp)

	latCfg, _ := json.Marshal(models.LatencyConfig{
		Distribution:   models.DistUniform,
		JitterMinMs:    150,
		JitterMaxMs:    450,
		FixedLatencyMs: 250,
	})
	latRule := &models.FaultRule{
		ID:               "rul_pay_lat",
		ExperimentID:     "exp_blackfriday",
		Name:             "Payment Latency Jitter",
		FaultType:        models.FaultLatency,
		Probability:      0.80, // 80% of requests delayed
		Priority:         10,
		MatchMethods:     []string{"POST"},
		MatchPathPattern: "^/v1/charges",
		Config:           latCfg,
		IsEnabled:        true,
	}
	_ = s.CreateFaultRule(ctx, latRule)

	errCfg, _ := json.Marshal(models.HTTPErrorConfig{
		StatusCode:   http.StatusServiceUnavailable,
		ResponseBody: `{"error":"Payment Processor Overloaded","code":"PAYMENT_GATEWAY_THROTTLED"}`,
		PreUpstream:  true,
		Headers: map[string]string{
			"Retry-After": "15",
		},
	})
	errRule := &models.FaultRule{
		ID:           "rul_pay_err",
		ExperimentID: "exp_blackfriday",
		Name:         "Random Gateway 503 Spikes",
		FaultType:    models.FaultHTTPError,
		Probability:  0.15, // 15% random failure rate
		Priority:     20,
		MatchMethods: []string{"POST"},
		Config:       errCfg,
		IsEnabled:    true,
	}
	_ = s.CreateFaultRule(ctx, errRule)
}

func createWebHandler(staticDir string) http.HandlerFunc {
	// Check if built web directory exists
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		return func(w http.ResponseWriter, r *http.Request) {
			path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// Fallback to index.html for React SPA client routing
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, r)
		}
	}

	// Dynamic fallback landing page if static files aren't built yet
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>FaultLine | Chaos Reverse Proxy</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0f19; color: #f3f4f6; margin: 0; padding: 40px; }
    .card { background: #111827; border: 1px solid #1f2937; border-radius: 12px; padding: 24px; max-width: 800px; margin: 0 auto; box-shadow: 0 8px 24px rgba(0,0,0,0.5); }
    h1 { color: #f43f5e; margin-top: 0; font-size: 26px; display: flex; align-items: center; gap: 10px; }
    .badge { background: #374151; color: #9ca3af; font-size: 12px; padding: 3px 8px; border-radius: 6px; font-weight: normal; }
    pre { background: #030712; padding: 12px; border-radius: 8px; overflow-x: auto; color: #38bdf8; font-size: 13px; }
    a { color: #f43f5e; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .btn { display: inline-block; background: #f43f5e; color: #fff; padding: 10px 18px; border-radius: 6px; font-weight: 600; text-decoration: none; margin-top: 16px; }
  </style>
</head>
<body>
  <div class="card">
    <h1>⚡ FaultLine <span class="badge">Proxy Online</span></h1>
    <p>Security-First Programmable Chaos Engineering Reverse Proxy engine is actively running.</p>
    <h3>Active Core Endpoints:</h3>
    <ul>
      <li><a href="/api/v1/health">/api/v1/health</a> — Service & API Health Status</li>
      <li><a href="/api/v1/upstreams">/api/v1/upstreams</a> — Registered Upstream Catalog</li>
      <li><a href="/api/v1/experiments">/api/v1/experiments</a> — Active Chaos Experiments</li>
      <li><a href="/api/v1/metrics/live">/api/v1/metrics/live</a> — Real-Time Proxy Telemetry</li>
      <li><a href="/metrics">/metrics</a> — Prometheus Metrics Scraper</li>
      <li><code>/proxy/{upstream_slug}/*</code> — Reverse Proxy Ingress Route</li>
    </ul>
    <h3>Quick Test (cURL):</h3>
    <pre>curl -X POST http://localhost:8080/proxy/payments/v1/charges \
  -H "Content-Type: application/json" \
  -d '{"amount": 99.00}'</pre>
  </div>
</body>
</html>`))
	}
}
