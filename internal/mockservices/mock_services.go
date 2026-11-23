package mockservices

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// MockCluster runs local mock upstream microservices for live chaos testing.
type MockCluster struct {
	OrderServer     *httptest.Server
	PaymentServer   *httptest.Server
	InventoryServer *httptest.Server

	mu        sync.Mutex
	orderReqs int64
}

// Start launches simulated microservices on ephemeral local ports.
func Start() *MockCluster {
	cluster := &MockCluster{}

	// 1. Order Service Mock
	orderMux := http.NewServeMux()
	orderMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"order-service","version":"2.4.1"}`))
	})
	orderMux.HandleFunc("POST /v1/orders", func(w http.ResponseWriter, r *http.Request) {
		cluster.mu.Lock()
		cluster.orderReqs++
		num := cluster.orderReqs
		cluster.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id":   fmt.Sprintf("ord_live_%d", num),
			"status":     "confirmed",
			"total_usd":  149.99,
			"items":      3,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		})
	})
	orderHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id": "ord_sample_987",
			"customer": "enterprise-client@acme.corp",
			"status":   "shipped",
			"carrier":  "FedEx Priority",
		})
	}
	orderMux.HandleFunc("GET /v1/orders", orderHandler)
	orderMux.HandleFunc("GET /v1/orders/", orderHandler)
	cluster.OrderServer = httptest.NewServer(orderMux)

	// 2. Payment Gateway Mock
	payMux := http.NewServeMux()
	payMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"HEALTHY","gateway":"payment-processor","status_code":200}`))
	})
	payMux.HandleFunc("POST /v1/charges", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"charge_id": "chg_9921_auth",
			"amount":    14999,
			"currency":  "USD",
			"paid":      true,
			"receipt":   "https://receipts.paymock.internal/rcpt_9921",
		})
	})
	cluster.PaymentServer = httptest.NewServer(payMux)

	// 3. Inventory Service Mock
	invMux := http.NewServeMux()
	invMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ONLINE","service":"inventory-service"}`))
	})
	stockHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sku":        "SKU-TURBO-77",
			"available":  420,
			"warehouse":  "US-EAST-1",
			"backorder":  false,
		})
	}
	invMux.HandleFunc("GET /v1/stock", stockHandler)
	invMux.HandleFunc("GET /v1/stock/", stockHandler)
	cluster.InventoryServer = httptest.NewServer(invMux)

	return cluster
}

// Close terminates all mock server instances.
func (c *MockCluster) Close() {
	if c.OrderServer != nil {
		c.OrderServer.Close()
	}
	if c.PaymentServer != nil {
		c.PaymentServer.Close()
	}
	if c.InventoryServer != nil {
		c.InventoryServer.Close()
	}
}
