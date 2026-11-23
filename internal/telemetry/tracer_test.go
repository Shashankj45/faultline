package telemetry

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestTracerInitAndSpan(t *testing.T) {
	_, shutdown := InitTracer("faultline-test")
	defer func() {
		_ = shutdown(context.Background())
	}()

	req := httptest.NewRequest("GET", "/proxy/orders/v1/orders", nil)
	ctx, span := StartProxySpan(req, "orders")
	defer span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	// Verify W3C traceparent was injected into outbound headers
	traceparent := req.Header.Get("traceparent")
	if traceparent == "" {
		t.Error("expected traceparent header to be injected into request headers")
	}

	// Verify RecordFaultInjection doesn't panic
	RecordFaultInjection(span, "latency", "exp_123", 0.5)
}
