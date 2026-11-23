package faults

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
	"golang.org/x/time/rate"
)

var (
	ErrRequestAborted = errors.New("fault injection: client aborted connection")
)

// Engine coordinates chaos injections across incoming and outgoing streams.
type Engine struct {
	matcher *Matcher
	burstMu sync.Mutex
}

// NewEngine creates a new Fault Injection Engine.
func NewEngine() *Engine {
	return &Engine{
		matcher: NewMatcher(),
	}
}

// Matcher returns the internal matcher.
func (e *Engine) Matcher() *Matcher {
	return e.matcher
}

// ApplyLatency injects artificial latency according to LatencyConfig.
func (e *Engine) ApplyLatency(ctx context.Context, cfg *models.LatencyConfig, expStartedAt *time.Time) error {
	var delay time.Duration

	switch cfg.Distribution {
	case models.DistFixed:
		delay = time.Duration(cfg.FixedLatencyMs) * time.Millisecond

	case models.DistUniform:
		minMs := cfg.JitterMinMs
		maxMs := cfg.JitterMaxMs
		if maxMs < minMs {
			maxMs = minMs
		}
		jitter := minMs
		if maxMs > minMs {
			jitter += rand.Intn(maxMs - minMs + 1)
		}
		delay = time.Duration(jitter) * time.Millisecond

	case models.DistGaussian:
		// Box-Muller transform for normal distribution
		u1 := CryptoFloat64()
		u2 := CryptoFloat64()
		if u1 <= 0.0 {
			u1 = 0.0001
		}
		z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
		val := cfg.MeanMs + z*cfg.StdDevMs
		if val < 0 {
			val = 0
		}
		delay = time.Duration(val) * time.Millisecond

	case models.DistRamp:
		// Gradual latency increase over time
		startMs := float64(cfg.RampStartMs)
		endMs := float64(cfg.RampEndMs)
		rampDuration := float64(cfg.RampDurationSeconds)
		if rampDuration <= 0 {
			rampDuration = 60.0
		}

		elapsedSec := 0.0
		if expStartedAt != nil {
			elapsedSec = time.Since(*expStartedAt).Seconds()
		}

		progress := elapsedSec / rampDuration
		if progress > 1.0 {
			progress = 1.0
		}
		currentMs := startMs + (endMs-startMs)*progress
		delay = time.Duration(currentMs) * time.Millisecond

	default:
		if cfg.FixedLatencyMs > 0 {
			delay = time.Duration(cfg.FixedLatencyMs) * time.Millisecond
		}
	}

	if delay <= 0 {
		return nil
	}

	// Context-aware sleep: immediately abort if client disconnects
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ErrRequestAborted
	case <-timer.C:
		return nil
	}
}

// InjectHTTPError writes a simulated HTTP error response directly to the client.
func (e *Engine) InjectHTTPError(w http.ResponseWriter, cfg *models.HTTPErrorConfig) {
	statusCode := cfg.StatusCode
	if statusCode < 100 || statusCode > 599 {
		statusCode = http.StatusInternalServerError
	}

	for k, v := range cfg.Headers {
		w.Header().Set(k, v)
	}

	w.Header().Set("X-FaultLine-Injected", "true")
	w.Header().Set("X-FaultLine-Fault", "http_error")

	// Set content-type if not present
	if w.Header().Get("Content-Type") == "" {
		if json.Valid([]byte(cfg.ResponseBody)) {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
	}

	w.WriteHeader(statusCode)
	if cfg.ResponseBody != "" {
		_, _ = w.Write([]byte(cfg.ResponseBody))
	} else {
		_, _ = fmt.Fprintf(w, `{"error":"Injected Chaos Error","status":%d,"fault":"http_error"}`, statusCode)
	}
}

// InjectConnectionDrop hijacks the underlying TCP socket to simulate abrupt connection failures.
func (e *Engine) InjectConnectionDrop(w http.ResponseWriter, r *http.Request, cfg *models.ConnectionDropConfig) error {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("response writer does not support connection hijacking")
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("hijack failed: %w", err)
	}
	defer conn.Close()

	switch cfg.DropType {
	case models.DropTCPReset:
		// Attempt to set SO_LINGER to 0 to send a TCP RST packet
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetLinger(0)
		}
		// Closing immediately sends RST on TCP sockets with SetLinger(0)
		return nil

	case models.DropEmptyEOF:
		// Instant silent close without sending any bytes
		return nil

	case models.DropPartialBody:
		// Send HTTP 200 OK headers + partial body, then abruptly cut connection
		partial := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1048576\r\n\r\n{\"status\":\"partial_stream_drop\","
		if cfg.PartialByteCount > 0 && cfg.PartialByteCount < len(partial) {
			partial = partial[:cfg.PartialByteCount]
		}
		_, _ = bufrw.WriteString(partial)
		_ = bufrw.Flush()
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetLinger(0)
		}
		return nil

	case models.DropMalformedBytes:
		// Write garbage protocol bytes
		_, _ = bufrw.WriteString("INVALID_HTTP_PROTOCOL_CHAOS_BYTE_SEQUENCE_1024_CORRUPTION\r\n\r\n")
		_ = bufrw.Flush()
		return nil

	default:
		// Default to TCP reset
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetLinger(0)
		}
		return nil
	}
}

// InjectTimeout handles slow-burn timeout scenarios (stalls, slowloris, drip-feed, gateway timeouts).
func (e *Engine) InjectTimeout(ctx context.Context, w http.ResponseWriter, cfg *models.TimeoutConfig) error {
	switch cfg.TimeoutType {
	case models.TimeoutClientStall:
		// Stall until client gives up
		select {
		case <-ctx.Done():
			return ErrRequestAborted
		case <-time.After(60 * time.Second): // Safety cap
			return nil
		}

	case models.TimeoutGatewayTimeout:
		delay := time.Duration(cfg.DelayMs) * time.Millisecond
		if delay <= 0 {
			delay = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return ErrRequestAborted
		case <-time.After(delay):
			w.Header().Set("X-FaultLine-Injected", "true")
			w.Header().Set("X-FaultLine-Fault", "gateway_timeout")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte(`{"error":"Gateway Timeout","status":504,"fault":"simulated_gateway_timeout"}`))
			return nil
		}

	case models.TimeoutSlowWrite:
		// Drip-feed response byte by byte
		flusher, ok := w.(http.Flusher)
		w.Header().Set("X-FaultLine-Injected", "true")
		w.Header().Set("X-FaultLine-Fault", "slow_write")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		interval := time.Duration(cfg.ChunkIntervalMs) * time.Millisecond
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		chunkSize := cfg.ChunkSizeBytes
		if chunkSize <= 0 {
			chunkSize = 16
		}

		payload := []byte("FaultLine Drip-Feed Throttling Chaos Test Payload Stream...")
		for i := 0; i < len(payload); i += chunkSize {
			end := i + chunkSize
			if end > len(payload) {
				end = len(payload)
			}
			select {
			case <-ctx.Done():
				return ErrRequestAborted
			case <-time.After(interval):
				_, _ = w.Write(payload[i:end])
				if ok {
					flusher.Flush()
				}
			}
		}
		return nil

	default:
		return nil
	}
}

// WrapThrottledReader wraps an io.Reader (like request body) with a token-bucket rate limiter.
func WrapThrottledReader(ctx context.Context, r io.Reader, bytesPerSec int64) io.Reader {
	if bytesPerSec <= 0 {
		return r
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSec), int(bytesPerSec))
	return &throttledReader{
		ctx:     ctx,
		r:       r,
		limiter: limiter,
	}
}

type throttledReader struct {
	ctx     context.Context
	r       io.Reader
	limiter *rate.Limiter
}

func (tr *throttledReader) Read(p []byte) (int, error) {
	n, err := tr.r.Read(p)
	if n > 0 {
		ctx := tr.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if waitErr := tr.limiter.WaitN(ctx, n); waitErr != nil {
			return n, waitErr
		}
	}
	return n, err
}

// WrapThrottledWriter wraps an io.Writer with a token-bucket rate limiter.
func WrapThrottledWriter(ctx context.Context, w io.Writer, bytesPerSec int64) io.Writer {
	if bytesPerSec <= 0 {
		return w
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSec), int(bytesPerSec))
	return &throttledWriter{
		ctx:     ctx,
		w:       w,
		limiter: limiter,
	}
}

type throttledWriter struct {
	ctx     context.Context
	w       io.Writer
	limiter *rate.Limiter
}

func (tw *throttledWriter) Write(p []byte) (int, error) {
	ctx := tw.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := tw.limiter.WaitN(ctx, len(p)); err != nil {
		return 0, err
	}
	return tw.w.Write(p)
}

// --- Packet Loss Simulation ---

// packetLossWriter wraps an io.Writer and stochastically drops or corrupts chunks.
// Since we operate at the HTTP layer (not actual IP), we simulate packet loss by:
//  1. Dropping chunks entirely (not writing them) — produces garbled/truncated body.
//  2. Corrupting random bytes within a chunk — simulates bitflip/checksum failure.
//
// This is applied to the downstream response writer so the client sees a corrupted stream.
type packetLossWriter struct {
	w            io.Writer
	dropPercent  float64 // 0.0–1.0
	corruptBytes bool
}

func (pl *packetLossWriter) Write(p []byte) (int, error) {
	if pl.dropPercent > 0 && CryptoFloat64() < pl.dropPercent {
		// Silently swallow this chunk — simulates dropped IP packet segment
		return len(p), nil
	}
	if pl.corruptBytes && len(p) > 0 {
		// Clone to avoid mutating caller's buffer
		mutated := make([]byte, len(p))
		copy(mutated, p)
		// Corrupt a random byte
		idx := int(CryptoFloat64() * float64(len(mutated)))
		if idx >= len(mutated) {
			idx = len(mutated) - 1
		}
		mutated[idx] ^= 0xFF // Flip all bits in one byte
		return pl.w.Write(mutated)
	}
	return pl.w.Write(p)
}

// WrapPacketLossWriter wraps a downstream ResponseWriter body with packet loss simulation.
// Returns the writer unmodified if no loss is configured.
func WrapPacketLossWriter(w io.Writer, cfg *models.PacketLossConfig) io.Writer {
	if cfg == nil {
		return w
	}
	if cfg.DropChunkPercent <= 0 && !cfg.CorruptBytes {
		return w
	}
	drop := cfg.DropChunkPercent
	if drop > 1.0 {
		drop = 1.0
	}
	return &packetLossWriter{
		w:            w,
		dropPercent:  drop,
		corruptBytes: cfg.CorruptBytes,
	}
}

// IsBurstActive evaluates if the current time falls inside an active burst failure window.
func IsBurstActive(cfg *models.BurstConfig) bool {
	if cfg.BurstIntervalMs <= 0 || cfg.BurstDurationMs <= 0 {
		return false
	}
	nowMs := time.Now().UnixMilli()
	cyclePosition := nowMs % int64(cfg.BurstIntervalMs)
	return cyclePosition < int64(cfg.BurstDurationMs)
}
