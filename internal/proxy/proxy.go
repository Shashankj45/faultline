package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/faultline-proxy/faultline/internal/faults"
	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/internal/telemetry"
	"github.com/faultline-proxy/faultline/internal/validator"
	"github.com/faultline-proxy/faultline/pkg/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ReverseProxyServer orchestrates dynamic routing, SSRF enforcement, and fault injection.
type ReverseProxyServer struct {
	store      *store.SQLStore
	engine     *faults.Engine
	metrics    *metrics.Metrics
	bufferPool httputil.BufferPool

	// Per-upstream reverse proxy cache
	proxiesMu sync.RWMutex
	proxies   map[string]*httputil.ReverseProxy // key: upstream_id
}

// NewReverseProxyServer creates a reverse proxy server instance.
func NewReverseProxyServer(s *store.SQLStore, engine *faults.Engine, m *metrics.Metrics) *ReverseProxyServer {
	return &ReverseProxyServer{
		store:      s,
		engine:     engine,
		metrics:    m,
		bufferPool: NewBytePool(32 * 1024),
		proxies:    make(map[string]*httputil.ReverseProxy),
	}
}

// ServeHTTP handles incoming reverse proxy traffic.
func (p *ReverseProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// 1. Resolve Target Upstream
	upstream, strippedPath, ok := p.resolveUpstream(r)
	if !ok || !upstream.IsActive {
		http.Error(w, `{"error":"Upstream service not registered or inactive"}`, http.StatusNotFound)
		return
	}

	// Start OpenTelemetry span with W3C TraceContext propagation
	reqCtx, span := telemetry.StartProxySpan(r, upstream.Slug)
	defer span.End()
	r = r.WithContext(reqCtx)

	clientIP := getClientIP(r)

	// 2. Check Emergency Kill Switch (O(1) in-memory check)
	killSwitch := p.store.CachedKillSwitch()
	if killSwitch.GlobalActive {
		span.SetAttributes(attribute.Bool("chaos.kill_switch_active", true))
		// Passthrough immediately without chaos
		p.forwardRequest(w, r, upstream, strippedPath, nil, start)
		return
	}

	// 3. Find Matching Fault Rules
	activeRules := p.store.CachedActiveRules(upstream.ID)
	var matchedRule *models.FaultRule

	for i := range activeRules {
		rule := &activeRules[i]
		if p.engine.Matcher().Matches(r, clientIP, rule) {
			matchedRule = rule
			break // Apply highest priority matched rule
		}
	}

	// 4. Pre-Upstream Fault Interceptors
	if matchedRule != nil {
		telemetry.RecordFaultInjection(span, string(matchedRule.FaultType), matchedRule.ExperimentID, matchedRule.Probability)
		injected := p.executePreUpstreamFault(w, r, upstream, strippedPath, matchedRule, start)
		if injected {
			// Fault completely handled response (e.g. error, connection drop, stall)
			return
		}
	}

	// 5. Forward to Upstream
	p.forwardRequest(w, r, upstream, strippedPath, matchedRule, start)
}

func (p *ReverseProxyServer) executePreUpstreamFault(w http.ResponseWriter, r *http.Request, upstream *models.Upstream, strippedPath string, rule *models.FaultRule, start time.Time) bool {
	ctx := r.Context()

	switch rule.FaultType {
	case models.FaultLatency:
		var cfg models.LatencyConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultLatency))
			// Fetch experiment's started_at for accurate ramp latency calculation
			var expStartedAt *time.Time
			if cfg.Distribution == models.DistRamp {
				if exp, err := p.store.GetExperimentByID(ctx, rule.ExperimentID); err == nil {
					expStartedAt = exp.StartedAt
				}
			}
			_ = p.engine.ApplyLatency(ctx, &cfg, expStartedAt)
		}
		return false // Continue to upstream after latency

	case models.FaultHTTPError:
		var cfg models.HTTPErrorConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			if cfg.PreUpstream {
				p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultHTTPError))
				p.engine.InjectHTTPError(w, &cfg)
				p.metrics.RecordRequest(upstream.Slug, r.Method, strconv.Itoa(cfg.StatusCode), time.Since(start), string(models.FaultHTTPError))
				return true
			}
		}
		return false

	case models.FaultConnectionDrop:
		var cfg models.ConnectionDropConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultConnectionDrop))
			_ = p.engine.InjectConnectionDrop(w, r, &cfg)
			p.metrics.RecordRequest(upstream.Slug, r.Method, "000_DROP", time.Since(start), string(models.FaultConnectionDrop))
			return true
		}
		return false

	case models.FaultTimeout:
		var cfg models.TimeoutConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultTimeout))
			_ = p.engine.InjectTimeout(ctx, w, &cfg)
			p.metrics.RecordRequest(upstream.Slug, r.Method, "504_TIMEOUT", time.Since(start), string(models.FaultTimeout))
			return true
		}
		return false

	case models.FaultBandwidthLimit:
		var cfg models.BandwidthConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			if cfg.Direction == models.BandwidthUpstream && r.Body != nil {
				p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultBandwidthLimit))
				r.Body = io.NopCloser(faults.WrapThrottledReader(r.Body, cfg.RateBytesPerSec))
			}
		}
		return false

	case models.FaultPacketLoss:
		var cfg models.PacketLossConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultPacketLoss))
			// Wrap the ResponseWriter so the downstream response stream gets dropped/corrupted chunks
			pw := &packetLossResponseWriter{ResponseWriter: w, cfg: &cfg}
			w.Header().Set("X-FaultLine-Injected", "true")
			w.Header().Set("X-FaultLine-Fault", "packet_loss")
			p.forwardRequest(pw, r, upstream, strippedPath, rule, start)
			return true
		}
		return false

	case models.FaultBurst:
		var cfg models.BurstConfig
		if err := json.Unmarshal(rule.Config, &cfg); err == nil {
			if faults.IsBurstActive(&cfg) {
				p.metrics.RecordInjectedFault(upstream.Slug, rule.ExperimentID, string(models.FaultBurst))
				statusCode := cfg.BurstStatusCode
				if statusCode == 0 {
					statusCode = http.StatusServiceUnavailable
				}
				errCfg := &models.HTTPErrorConfig{
					StatusCode:   statusCode,
					ResponseBody: fmt.Sprintf(`{"error":"Burst Failure Simulation Spike","status":%d,"fault":"burst"}`, statusCode),
					PreUpstream:  true,
				}
				p.engine.InjectHTTPError(w, errCfg)
				p.metrics.RecordRequest(upstream.Slug, r.Method, strconv.Itoa(statusCode), time.Since(start), string(models.FaultBurst))
				return true
			}
		}
		return false

	default:
		return false
	}
}

func (p *ReverseProxyServer) forwardRequest(w http.ResponseWriter, r *http.Request, upstream *models.Upstream, path string, matchedRule *models.FaultRule, start time.Time) {
	proxy := p.getOrCreateReverseProxy(upstream)

	// Adjust request path
	r.URL.Path = path
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")
	if r.TLS != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	}

	// Wrap ResponseWriter to capture status code and latency
	rec := &statusRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	proxy.ServeHTTP(rec, r)

	duration := time.Since(start)
	faultName := "none"
	if matchedRule != nil {
		faultName = string(matchedRule.FaultType)
	}

	if span := trace.SpanFromContext(r.Context()); span != nil && span.IsRecording() {
		span.SetAttributes(
			attribute.Int("http.response.status_code", rec.statusCode),
			attribute.Int64("http.response.duration_ms", duration.Milliseconds()),
			attribute.String("chaos.applied_fault", faultName),
		)
	}

	p.metrics.RecordRequest(upstream.Slug, r.Method, strconv.Itoa(rec.statusCode), duration, faultName)
}

func (p *ReverseProxyServer) getOrCreateReverseProxy(upstream *models.Upstream) *httputil.ReverseProxy {
	p.proxiesMu.RLock()
	proxy, ok := p.proxies[upstream.ID]
	p.proxiesMu.RUnlock()
	if ok {
		return proxy
	}

	p.proxiesMu.Lock()
	defer p.proxiesMu.Unlock()

	// Double check
	if proxy, ok = p.proxies[upstream.ID]; ok {
		return proxy
	}

	targetURL, _ := url.Parse(upstream.TargetURL)
	timeout := time.Duration(upstream.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	transport := &http.Transport{
		DialContext:           validator.SafeDialer(upstream, timeout).DialContext,
		MaxIdleConns:          upstream.MaxIdleConns,
		MaxIdleConnsPerHost:   upstream.MaxIdleConns,
		IdleConnTimeout:       time.Duration(upstream.IdleConnTimeoutSeconds) * time.Second,
		ResponseHeaderTimeout: time.Duration(upstream.ResponseHeaderTimeoutSec) * time.Second,
		DisableCompression:    true,
	}

	revProxy := httputil.NewSingleHostReverseProxy(targetURL)
	revProxy.Transport = transport
	revProxy.BufferPool = p.bufferPool
	revProxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-FaultLine-Proxy", "1.0")
		resp.Header.Set("X-FaultLine-Upstream", upstream.Slug)

		// Rewrite relative Location headers on upstream redirects so clients remain within the proxy route
		if loc := resp.Header.Get("Location"); loc != "" {
			if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/proxy/") {
				resp.Header.Set("Location", fmt.Sprintf("/proxy/%s%s", upstream.Slug, loc))
			}
		}
		return nil
	}
	revProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":"Upstream Gateway Error","details":%q,"status":502}`, err.Error())
	}

	p.proxies[upstream.ID] = revProxy
	return revProxy
}

func (p *ReverseProxyServer) resolveUpstream(r *http.Request) (*models.Upstream, string, bool) {
	// 1. Path-based routing: /proxy/{slug}/...
	path := r.URL.Path
	if strings.HasPrefix(path, "/proxy/") {
		parts := strings.SplitN(strings.TrimPrefix(path, "/proxy/"), "/", 2)
		slug := parts[0]
		remaining := "/"
		if len(parts) > 1 {
			remaining = "/" + parts[1]
		}
		if u, ok := p.store.CachedUpstreamBySlug(slug); ok {
			return u, remaining, true
		}
	}

	// 2. Header-based routing: X-FaultLine-Upstream: {slug}
	if slug := r.Header.Get("X-FaultLine-Upstream"); slug != "" {
		if u, ok := p.store.CachedUpstreamBySlug(slug); ok {
			return u, path, true
		}
	}

	return nil, path, false
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return ip
	}
	return r.RemoteAddr
}

// packetLossResponseWriter wraps a ResponseWriter and applies packet loss to the response body.
type packetLossResponseWriter struct {
	http.ResponseWriter
	cfg        *models.PacketLossConfig
	plWriter   io.Writer
	wroteHdr   bool
	statusCode int
}

func (pl *packetLossResponseWriter) WriteHeader(code int) {
	if !pl.wroteHdr {
		pl.statusCode = code
		pl.wroteHdr = true
		pl.ResponseWriter.WriteHeader(code)
	}
}

func (pl *packetLossResponseWriter) Write(b []byte) (int, error) {
	if !pl.wroteHdr {
		pl.WriteHeader(http.StatusOK)
	}
	if pl.plWriter == nil {
		pl.plWriter = faults.WrapPacketLossWriter(pl.ResponseWriter, pl.cfg)
	}
	return pl.plWriter.Write(b)
}

func (pl *packetLossResponseWriter) Flush() {
	if f, ok := pl.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	wroteHdr   bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHdr {
		r.statusCode = code
		r.wroteHdr = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHdr {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Hijack delegates to underlying ResponseWriter if supported
func (r *statusRecorder) Hijack() (net.Conn, *bytes.Buffer, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		c, rw, err := h.Hijack()
		if err != nil {
			return nil, nil, err
		}
		// Convert bufio to bytes.Buffer or return connection
		return c, bytes.NewBuffer(nil), rw.Flush()
	}
	return nil, nil, errors.New("hijack not supported")
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
