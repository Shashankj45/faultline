package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/faultline-proxy/faultline/internal/audit"
	"github.com/faultline-proxy/faultline/internal/auth"
	"github.com/faultline-proxy/faultline/internal/health"
	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/internal/proxy"
	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/internal/validator"
	"github.com/faultline-proxy/faultline/pkg/models"
	"github.com/google/uuid"
)

// Router orchestrates the Admin REST API.
type Router struct {
	mux         *http.ServeMux
	store       *store.SQLStore
	proxyServer *proxy.ReverseProxyServer
	healthMon   *health.Monitor
	metrics     *metrics.Metrics
	auditLogger *audit.Logger
	authService *auth.AuthService
	validator   *validator.DestinationValidator
	// devMode disables API key enforcement when no keys exist yet (bootstrap mode).
	// Once any key is created, enforcement is active on all subsequent requests.
	devMode bool
}

// NewRouter constructs the API router and registers endpoints.
func NewRouter(
	s *store.SQLStore,
	p *proxy.ReverseProxyServer,
	hm *health.Monitor,
	m *metrics.Metrics,
	al *audit.Logger,
	as *auth.AuthService,
) *Router {
	r := &Router{
		mux:         http.NewServeMux(),
		store:       s,
		proxyServer: p,
		healthMon:   hm,
		metrics:     m,
		auditLogger: al,
		authService: as,
		validator:   validator.NewDestinationValidator(nil),
	}
	r.registerRoutes()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Enable CORS for frontend dashboard
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-FaultLine-Upstream")

	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	r.mux.ServeHTTP(w, req)
}

type contextKey string

const actorContextKey contextKey = "faultline_actor"

func (r *Router) getActor(req *http.Request) string {
	if a, ok := req.Context().Value(actorContextKey).(string); ok && a != "" {
		return a
	}
	return "anonymous"
}

// requireRole wraps a handler with RBAC enforcement.
//
// Bootstrap mode: if no API keys exist in the store, the system is in dev/bootstrap mode
// and all requests are allowed (the console needs to be accessible to create the first key).
// Once at least one active key exists, every request to protected routes must carry a valid
// Bearer token or X-API-Key header with sufficient role.
func (r *Router) requireRole(required models.Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Check bootstrap mode via O(1) in-memory atomic flag.
		// If no active API keys exist, allow initial access so operator can bootstrap the first key.
		if !r.store.HasAPIKeys() {
			req = req.WithContext(context.WithValue(req.Context(), actorContextKey, "bootstrap"))
			next(w, req)
			return
		}

		apiKey, err := r.authService.Authenticate(req)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "unauthorized: missing or invalid API key (use X-API-Key header or Bearer token)")
			return
		}

		if !auth.EnforceRole(apiKey.Role, required) {
			jsonError(w, http.StatusForbidden, fmt.Sprintf("forbidden: role %q is required, your key has role %q", required, apiKey.Role))
			return
		}

		actorName := apiKey.Name
		if actorName == "" {
			actorName = apiKey.KeyPrefix
		}
		req = req.WithContext(context.WithValue(req.Context(), actorContextKey, actorName))
		next(w, req)
	}
}

func (r *Router) registerRoutes() {
	// Prometheus metrics endpoint (public)
	r.mux.Handle("/metrics", r.metrics.Handler())

	// API Health & Live Telemetry (public read)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/metrics/live", r.handleLiveMetrics)

	// Kill Switch (viewer can read; operator+ can write)
	r.mux.HandleFunc("GET /api/v1/killswitch", r.handleGetKillSwitch)
	r.mux.HandleFunc("POST /api/v1/killswitch", r.requireRole(models.RoleOperator, r.handleSetKillSwitch))

	// Upstreams (viewer can list/get; admin can mutate)
	r.mux.HandleFunc("GET /api/v1/upstreams", r.handleListUpstreams)
	r.mux.HandleFunc("POST /api/v1/upstreams", r.requireRole(models.RoleAdmin, r.handleCreateUpstream))
	r.mux.HandleFunc("GET /api/v1/upstreams/{id}", r.handleGetUpstream)
	r.mux.HandleFunc("PUT /api/v1/upstreams/{id}", r.requireRole(models.RoleAdmin, r.handleUpdateUpstream))
	r.mux.HandleFunc("DELETE /api/v1/upstreams/{id}", r.requireRole(models.RoleAdmin, r.handleDeleteUpstream))

	// Experiments (viewer can list/get; operator+ can mutate)
	r.mux.HandleFunc("GET /api/v1/experiments", r.handleListExperiments)
	r.mux.HandleFunc("POST /api/v1/experiments", r.requireRole(models.RoleOperator, r.handleCreateExperiment))
	r.mux.HandleFunc("GET /api/v1/experiments/{id}", r.handleGetExperiment)
	r.mux.HandleFunc("PUT /api/v1/experiments/{id}", r.requireRole(models.RoleOperator, r.handleUpdateExperiment))
	r.mux.HandleFunc("POST /api/v1/experiments/{id}/status", r.requireRole(models.RoleOperator, r.handleUpdateExperimentStatus))
	r.mux.HandleFunc("DELETE /api/v1/experiments/{id}", r.requireRole(models.RoleAdmin, r.handleDeleteExperiment))

	// Fault Rules (operator+ can mutate)
	r.mux.HandleFunc("GET /api/v1/experiments/{id}/rules", r.handleListRules)
	r.mux.HandleFunc("POST /api/v1/experiments/{id}/rules", r.requireRole(models.RoleOperator, r.handleCreateRule))
	r.mux.HandleFunc("PUT /api/v1/rules/{id}", r.requireRole(models.RoleOperator, r.handleUpdateRule))
	r.mux.HandleFunc("DELETE /api/v1/rules/{id}", r.requireRole(models.RoleAdmin, r.handleDeleteRule))

	// Audit Logs (viewer+)
	r.mux.HandleFunc("GET /api/v1/audit-logs", r.handleListAuditLogs)

	// API Keys (admin only)
	r.mux.HandleFunc("GET /api/v1/api-keys", r.requireRole(models.RoleAdmin, r.handleListAPIKeys))
	r.mux.HandleFunc("POST /api/v1/api-keys", r.requireRole(models.RoleAdmin, r.handleCreateAPIKey))
	r.mux.HandleFunc("DELETE /api/v1/api-keys/{id}", r.requireRole(models.RoleAdmin, r.handleRevokeAPIKey))

	// Interactive Test Studio (operator+)
	r.mux.HandleFunc("POST /api/v1/test-studio/dispatch", r.requireRole(models.RoleOperator, r.handleTestStudioDispatch))
}

// --- Handler Implementations ---

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"service":   "FaultLine Chaos Reverse Proxy",
		"timestamp": time.Now().UTC(),
	})
}

func (r *Router) handleLiveMetrics(w http.ResponseWriter, req *http.Request) {
	stats := r.metrics.GetLiveStats()
	jsonResponse(w, http.StatusOK, stats)
}

func (r *Router) handleGetKillSwitch(w http.ResponseWriter, req *http.Request) {
	ks, err := r.store.GetKillSwitch(req.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, ks)
}

func (r *Router) handleSetKillSwitch(w http.ResponseWriter, req *http.Request) {
	var input models.KillSwitchState
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	if input.ActivatedBy == "" {
		input.ActivatedBy = "web-console"
	}

	if err := r.store.SetKillSwitch(req.Context(), &input); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.metrics.SetKillSwitchStatus(input.GlobalActive)
	r.auditLogger.Log(req.Context(), input.ActivatedBy, "TOGGLE_KILL_SWITCH", "system", "kill_switch", input, req.RemoteAddr)

	jsonResponse(w, http.StatusOK, input)
}

// --- Upstreams Handlers ---

func (r *Router) handleListUpstreams(w http.ResponseWriter, req *http.Request) {
	upstreams, err := r.store.ListUpstreams(req.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Attach health status
	type UpstreamResponse struct {
		models.Upstream
		Health *models.UpstreamHealth `json:"health,omitempty"`
	}

	resp := make([]UpstreamResponse, len(upstreams))
	for i, u := range upstreams {
		resp[i].Upstream = u
		if h, ok := r.healthMon.GetStatus(u.ID); ok {
			resp[i].Health = h
		}
	}

	jsonResponse(w, http.StatusOK, resp)
}

func (r *Router) handleCreateUpstream(w http.ResponseWriter, req *http.Request) {
	var u models.Upstream
	if err := json.NewDecoder(req.Body).Decode(&u); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid upstream json payload")
		return
	}

	if u.Name == "" || u.Slug == "" || u.TargetURL == "" {
		jsonError(w, http.StatusBadRequest, "name, slug, and target_url are required")
		return
	}

	// Strict SSRF Validation
	_, err := r.validator.ValidateTarget(req.Context(), u.TargetURL, u.AllowLocalhost, u.AllowPrivateNetwork)
	if err != nil {
		jsonError(w, http.StatusUnprocessableEntity, fmt.Sprintf("SSRF / Destination Validation Blocked: %v", err))
		return
	}

	if u.ID == "" {
		u.ID = fmt.Sprintf("ups_%s", uuid.New().String()[:8])
	}
	if u.TimeoutSeconds <= 0 {
		u.TimeoutSeconds = 30
	}
	if u.MaxIdleConns <= 0 {
		u.MaxIdleConns = 100
	}
	if u.IdleConnTimeoutSeconds <= 0 {
		u.IdleConnTimeoutSeconds = 90
	}
	if u.ResponseHeaderTimeoutSec <= 0 {
		u.ResponseHeaderTimeoutSec = 30
	}
	if u.HealthCheckPath == "" {
		u.HealthCheckPath = "/healthz"
	}
	if u.HealthCheckIntervalSeconds <= 0 {
		u.HealthCheckIntervalSeconds = 10
	}
	u.IsActive = true

	if err := r.store.CreateUpstream(req.Context(), &u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			jsonError(w, http.StatusConflict, "upstream slug already exists")
			return
		}
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "CREATE_UPSTREAM", "upstream", u.ID, u, req.RemoteAddr)
	jsonResponse(w, http.StatusCreated, u)
}

func (r *Router) handleGetUpstream(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	u, err := r.store.GetUpstreamByID(req.Context(), id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "upstream not found")
		return
	}
	jsonResponse(w, http.StatusOK, u)
}

func (r *Router) handleUpdateUpstream(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var u models.Upstream
	if err := json.NewDecoder(req.Body).Decode(&u); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid upstream json payload")
		return
	}
	u.ID = id

	// Validate target
	_, err := r.validator.ValidateTarget(req.Context(), u.TargetURL, u.AllowLocalhost, u.AllowPrivateNetwork)
	if err != nil {
		jsonError(w, http.StatusUnprocessableEntity, fmt.Sprintf("SSRF Validation Failed: %v", err))
		return
	}

	if err := r.store.UpdateUpstream(req.Context(), &u); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate cached reverse proxy to apply new host/transport configs
	r.proxyServer.EvictProxy(u.ID)

	r.auditLogger.Log(req.Context(), r.getActor(req), "UPDATE_UPSTREAM", "upstream", u.ID, u, req.RemoteAddr)
	jsonResponse(w, http.StatusOK, u)
}

func (r *Router) handleDeleteUpstream(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.store.DeleteUpstream(req.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate cached reverse proxy
	r.proxyServer.EvictProxy(id)

	r.auditLogger.Log(req.Context(), r.getActor(req), "DELETE_UPSTREAM", "upstream", id, nil, req.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// --- Experiments Handlers ---

func (r *Router) handleListExperiments(w http.ResponseWriter, req *http.Request) {
	upstreamID := req.URL.Query().Get("upstream_id")
	exps, err := r.store.ListExperiments(req.Context(), upstreamID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, exps)
}

func (r *Router) handleCreateExperiment(w http.ResponseWriter, req *http.Request) {
	var exp models.Experiment
	if err := json.NewDecoder(req.Body).Decode(&exp); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid experiment json payload")
		return
	}

	if exp.UpstreamID == "" || exp.Name == "" {
		jsonError(w, http.StatusBadRequest, "upstream_id and name are required")
		return
	}

	if exp.ID == "" {
		exp.ID = fmt.Sprintf("exp_%s", uuid.New().String()[:8])
	}
	if exp.Status == "" {
		exp.Status = models.StatusDraft
	}
	if exp.MaxDurationSeconds <= 0 {
		exp.MaxDurationSeconds = 3600 // 1 hour safety TTL
	}

	// Emergency Kill Switch check: cannot start active experiment while global kill switch is engaged
	if exp.Status == models.StatusRunning {
		if r.store.CachedKillSwitch().GlobalActive {
			jsonError(w, http.StatusConflict, "cannot launch active experiment while global emergency kill switch is engaged")
			return
		}
	}

	if err := r.store.CreateExperiment(req.Context(), &exp); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "CREATE_EXPERIMENT", "experiment", exp.ID, exp, req.RemoteAddr)
	jsonResponse(w, http.StatusCreated, exp)
}

func (r *Router) handleGetExperiment(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	exp, err := r.store.GetExperimentByID(req.Context(), id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "experiment not found")
		return
	}
	jsonResponse(w, http.StatusOK, exp)
}

func (r *Router) handleUpdateExperiment(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var exp models.Experiment
	if err := json.NewDecoder(req.Body).Decode(&exp); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid experiment json payload")
		return
	}
	exp.ID = id

	// Emergency Kill Switch check
	if exp.Status == models.StatusRunning {
		if r.store.CachedKillSwitch().GlobalActive {
			jsonError(w, http.StatusConflict, "cannot set experiment to running while global emergency kill switch is engaged")
			return
		}
	}

	if err := r.store.UpdateExperiment(req.Context(), &exp); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "UPDATE_EXPERIMENT", "experiment", exp.ID, exp, req.RemoteAddr)
	jsonResponse(w, http.StatusOK, exp)
}

func (r *Router) handleUpdateExperimentStatus(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var body struct {
		Status models.ExperimentStatus `json:"status"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid status payload")
		return
	}

	// Emergency Kill Switch check: block activation while engaged
	if body.Status == models.StatusRunning {
		if r.store.CachedKillSwitch().GlobalActive {
			jsonError(w, http.StatusConflict, "cannot activate experiment while global emergency kill switch is engaged")
			return
		}
	}

	if err := r.store.UpdateExperimentStatus(req.Context(), id, body.Status); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "CHANGE_EXPERIMENT_STATUS", "experiment", id, body, req.RemoteAddr)
	jsonResponse(w, http.StatusOK, map[string]string{"id": id, "status": string(body.Status)})
}

func (r *Router) handleDeleteExperiment(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.store.DeleteExperiment(req.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.auditLogger.Log(req.Context(), r.getActor(req), "DELETE_EXPERIMENT", "experiment", id, nil, req.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// --- Fault Rules Handlers ---

func (r *Router) handleListRules(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	rules, err := r.store.ListRulesByExperiment(req.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, rules)
}

func (r *Router) handleCreateRule(w http.ResponseWriter, req *http.Request) {
	expID := req.PathValue("id")
	var rule models.FaultRule
	if err := json.NewDecoder(req.Body).Decode(&rule); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid rule json payload")
		return
	}

	rule.ExperimentID = expID
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rul_%s", uuid.New().String()[:8])
	}
	if rule.Probability <= 0 {
		rule.Probability = 1.0
	}

	if err := r.store.CreateFaultRule(req.Context(), &rule); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "CREATE_RULE", "fault_rule", rule.ID, rule, req.RemoteAddr)
	jsonResponse(w, http.StatusCreated, rule)
}

func (r *Router) handleUpdateRule(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var rule models.FaultRule
	if err := json.NewDecoder(req.Body).Decode(&rule); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid rule json payload")
		return
	}
	rule.ID = id

	if err := r.store.UpdateFaultRule(req.Context(), &rule); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "UPDATE_RULE", "fault_rule", rule.ID, rule, req.RemoteAddr)
	jsonResponse(w, http.StatusOK, rule)
}

func (r *Router) handleDeleteRule(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.store.DeleteFaultRule(req.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.auditLogger.Log(req.Context(), r.getActor(req), "DELETE_RULE", "fault_rule", id, nil, req.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// --- Audit Logs Handler ---

func (r *Router) handleListAuditLogs(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	logs, err := r.store.ListAuditLogs(req.Context(), limit, offset)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, logs)
}

// --- API Keys Handlers ---

func (r *Router) handleListAPIKeys(w http.ResponseWriter, req *http.Request) {
	keys, err := r.store.ListAPIKeys(req.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, keys)
}

func (r *Router) handleCreateAPIKey(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Name string      `json:"name"`
		Role models.Role `json:"role"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid api key payload")
		return
	}
	if body.Name == "" {
		body.Name = "Default Key"
	}
	if body.Role == "" {
		body.Role = models.RoleViewer
	}

	rawKey, keyModel, err := auth.GenerateAPIKey(body.Name, body.Role, nil)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := r.store.CreateAPIKey(req.Context(), keyModel); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.auditLogger.Log(req.Context(), r.getActor(req), "CREATE_API_KEY", "api_key", keyModel.ID, keyModel, req.RemoteAddr)

	jsonResponse(w, http.StatusCreated, map[string]any{
		"api_key":   keyModel,
		"secret_key": rawKey, // Exposed only on generation
	})
}

func (r *Router) handleRevokeAPIKey(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.store.RevokeAPIKey(req.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.auditLogger.Log(req.Context(), r.getActor(req), "REVOKE_API_KEY", "api_key", id, nil, req.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// --- Interactive Test Studio Handler ---

func (r *Router) handleTestStudioDispatch(w http.ResponseWriter, req *http.Request) {
	var body struct {
		UpstreamSlug string            `json:"upstream_slug"`
		Method       string            `json:"method"`
		Path         string            `json:"path"`
		Headers      map[string]string `json:"headers"`
		Payload      string            `json:"payload"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid dispatch payload")
		return
	}

	if body.UpstreamSlug == "" {
		jsonError(w, http.StatusBadRequest, "upstream_slug is required")
		return
	}
	if body.Method == "" {
		body.Method = "GET"
	}
	if !strings.HasPrefix(body.Path, "/") {
		body.Path = "/" + body.Path
	}

	// Build simulated request
	proxyURL := fmt.Sprintf("/proxy/%s%s", body.UpstreamSlug, body.Path)
	var reqBody io.Reader
	if body.Payload != "" {
		reqBody = strings.NewReader(body.Payload)
	}
	simReq := httptest.NewRequest(body.Method, proxyURL, reqBody)
	for k, v := range body.Headers {
		simReq.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	start := time.Now()
	r.proxyServer.ServeHTTP(rec, simReq)
	duration := time.Since(start)

	resp := rec.Result()
	respBytes, _ := io.ReadAll(resp.Body)

	headerMap := make(map[string]string)
	for k, v := range resp.Header {
		headerMap[k] = strings.Join(v, ", ")
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"status_code":     resp.StatusCode,
		"status_text":     resp.Status,
		"duration_ms":     duration.Milliseconds(),
		"injected_fault":  resp.Header.Get("X-FaultLine-Fault"),
		"is_injected":     resp.Header.Get("X-FaultLine-Injected") == "true",
		"response_body":   string(respBytes),
		"response_headers": headerMap,
	})
}

// --- Response Helpers ---

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]any{
		"error":  msg,
		"status": status,
	})
}
