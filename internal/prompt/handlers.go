package prompt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// HTTPHandler provides HTTP handlers for prompt intelligence APIs.
type HTTPHandler struct {
	repo Repository

	// Canary state: maps capsuleID to canary version
	canaryVersions map[string]string
}

// NewHTTPHandler creates a new prompt HTTP handler.
func NewHTTPHandler(repo Repository) *HTTPHandler {
	return &HTTPHandler{
		repo:           repo,
		canaryVersions: make(map[string]string),
	}
}

// NewHTTPHandlerWithDB creates a new prompt HTTP handler from a *sql.DB.
func NewHTTPHandlerWithDB(db *sql.DB) *HTTPHandler {
	return NewHTTPHandler(NewRepository(db))
}

// RegisterRoutes registers all prompt intelligence routes on the given chi.Router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		// v1 prompt intelligence (for TiRouter plugin)
		r.Route("/v1/prompt", func(r chi.Router) {
			r.Post("/preflight", h.handlePreflight)
			r.Post("/feedback", h.handleFeedback)
			r.Get("/catalog/version", h.handleCatalogVersion)
		})

		// Prompt intelligence dashboard (T-013/014/015)
		r.Route("/prompts", func(r chi.Router) {
			r.Get("/", h.handleListCapsules)
			r.Get("/metrics", h.handleMetrics)
			r.Get("/{id}", h.handleGetCapsule)
			r.Get("/{id}/versions", h.handleGetVersions)
			r.Get("/{id}/versions/{version}", h.handleGetVersion)

			// Canary deployment
			r.Route("/{id}", func(r chi.Router) {
				r.Post("/canary", h.handleCanaryDeploy)
				r.Post("/promote", h.handlePromoteCanary)
				r.Post("/rollback", h.handleRollbackCanary)
			})

			// Observe (prompt observability)
			r.Post("/observe", h.handleObserve)
		})
	})
}

// --- Handlers ---

// POST /api/v1/prompt/preflight
func (h *HTTPHandler) handlePreflight(w http.ResponseWriter, r *http.Request) {
	var req PreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	maxCap := req.MaxCapsules
	if maxCap <= 0 {
		maxCap = 2
	}

	capsules, err := h.repo.ListCapsules(ctx, req.Intent, req.Domain, CapsuleStatusActive, maxCap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("query capsules: %v", err))
		return
	}

	resp := PreflightResponse{
		RequestID: fmt.Sprintf("req_%d", time.Now().UnixNano()),
		Reason:    fmt.Sprintf("retrieved %d capsules for intent=%q domain=%q", len(capsules), req.Intent, req.Domain),
	}

	if len(capsules) == 0 {
		resp.Decision = DecisionSkip
	} else {
		resp.Decision = DecisionUse
		resp.Capsules = make([]CapsuleEnvelope, 0, len(capsules))
		for _, c := range capsules {
			resp.Capsules = append(resp.Capsules, ToCapsuleEnvelope(c))
		}
	}

	// Record a trace
	if len(capsules) > 0 {
		start := time.Now()
		confidence := 0.9
		if len(capsules) > 1 {
			confidence = 0.8
		}
		trace := PromptTrace{
			ID:         fmt.Sprintf("trace_%d", time.Now().UnixNano()),
			RequestID:  resp.RequestID,
			CapsuleID:  capsules[0].ID,
			Decision:   resp.Decision,
			Confidence: confidence,
			ReasonCode: "preflight_match",
			LatencyMs:  int(time.Since(start).Milliseconds()),
			CreatedAt:  time.Now().Unix(),
		}
		_ = h.repo.CreateTrace(ctx, trace)
	}

	writeJSON(w, http.StatusOK, resp)
}

// POST /api/v1/prompt/feedback
func (h *HTTPHandler) handleFeedback(w http.ResponseWriter, r *http.Request) {
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RequestID == "" || req.CapsuleID == "" || req.Outcome == "" {
		writeError(w, http.StatusBadRequest, "request_id, capsule_id, and outcome are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	fb := PromptFeedback{
		ID:                fmt.Sprintf("fb_%d", time.Now().UnixNano()),
		RequestID:         req.RequestID,
		CapsuleID:         req.CapsuleID,
		Outcome:           req.Outcome,
		UserOverride:      req.UserOverride,
		AddedTokens:       req.AddedTokens,
		ProviderErrorCode: req.ProviderErrorCode,
		CreatedAt:         time.Now().Unix(),
	}

	if err := h.repo.CreateFeedback(ctx, fb); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create feedback: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": fb.ID, "status": "recorded"})
}

// GET /api/v1/prompt/catalog/version
func (h *HTTPHandler) handleCatalogVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	count, err := h.getCapsuleCount(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := CatalogVersionResponse{
		Version:   1,
		UpdatedAt: time.Now().Unix(),
		Count:     count,
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- T-013: Prompt Observe ---

// POST /api/prompts/observe
func (h *HTTPHandler) handleObserve(w http.ResponseWriter, r *http.Request) {
	var req ObserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RequestID == "" {
		writeError(w, http.StatusBadRequest, "request_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	decision := Decision(req.Decision)
	if decision == "" {
		decision = DecisionUse
	}

	trace := PromptTrace{
		ID:         fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		RequestID:  req.RequestID,
		CapsuleID:  req.CapsuleID,
		Decision:   decision,
		Confidence: req.Confidence,
		ReasonCode: req.ReasonCode,
		LatencyMs:  req.LatencyMs,
		CreatedAt:  time.Now().Unix(),
	}
	_ = h.repo.CreateTrace(ctx, trace)

	resp := ObserveResponse{
		RequestID: req.RequestID,
		Decision:  string(decision),
	}
	if req.CapsuleID != "" {
		resp.CapsuleID = req.CapsuleID
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- T-013: Get Prompt by ID ---

// GET /api/prompts/{id}
func (h *HTTPHandler) handleGetCapsule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "capsule id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	capsule, err := h.repo.GetCapsule(ctx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "capsule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := CapsuleEnvelope{
		ID:          capsule.ID,
		Name:        capsule.Name,
		Description: capsule.Description,
		Domain:      capsule.Domain,
		Intent:      splitIntent(capsule.Intent),
		Risk:        string(capsule.Risk),
		Status:      string(capsule.Status),
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- T-013: List all prompts ---

// GET /api/prompts
func (h *HTTPHandler) handleListCapsules(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := r.URL.Query().Get("status")
	var statusFilter CapsuleStatus
	if status != "" {
		statusFilter = CapsuleStatus(status)
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	intent := r.URL.Query().Get("intent")
	domain := r.URL.Query().Get("domain")

	capsules, err := h.repo.ListCapsules(ctx, intent, domain, statusFilter, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list capsules: %v", err))
		return
	}

	result := make([]CapsuleEnvelope, 0, len(capsules))
	for _, c := range capsules {
		result = append(result, ToCapsuleEnvelope(c))
	}

	writeJSON(w, http.StatusOK, result)
}

// --- T-013: Get versions ---

// GET /api/prompts/{id}/versions
func (h *HTTPHandler) handleGetVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	versions, err := h.repo.GetVersions(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, versions)
}

// GET /api/prompts/{id}/versions/{version}
func (h *HTTPHandler) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ver := chi.URLParam(r, "version")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	v, err := h.repo.GetVersion(ctx, id, ver)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, v)
}

// --- T-014: Canary Deployment ---

// POST /api/prompts/{id}/canary
func (h *HTTPHandler) handleCanaryDeploy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req CanaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Version == "" {
		writeError(w, http.StatusBadRequest, "version is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := h.repo.GetCapsule(ctx, id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "capsule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := h.repo.GetVersion(ctx, id, req.Version); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.canaryVersions[id] = req.Version

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "canary_deployed",
		"version": req.Version,
		"capsule": id,
	})
}

// POST /api/prompts/{id}/promote
func (h *HTTPHandler) handlePromoteCanary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req CanaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = CanaryRequest{}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if req.Version == "" {
		req.Version = h.canaryVersions[id]
	}
	if req.Version == "" {
		writeError(w, http.StatusBadRequest, "no canary version deployed for this capsule")
		return
	}

	if _, err := h.repo.GetCapsule(ctx, id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "capsule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = h.repo.UpdateCapsuleStatus(ctx, id, CapsuleStatusActive)
	delete(h.canaryVersions, id)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "promoted",
		"version": req.Version,
		"capsule": id,
	})
}

// POST /api/prompts/{id}/rollback
func (h *HTTPHandler) handleRollbackCanary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := h.repo.GetCapsule(ctx, id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "capsule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = h.repo.UpdateCapsuleStatus(ctx, id, CapsuleStatusDraft)
	delete(h.canaryVersions, id)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "rolled_back",
		"capsule": id,
	})
}

// --- T-015: Prompt Metrics ---

// GET /api/prompts/metrics
func (h *HTTPHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var m MetricsResponse

	capsules, err := h.repo.ListCapsules(ctx, "", "", "", 1000000)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	m.TotalCapsules = int64(len(capsules))

	for _, c := range capsules {
		versions, err := h.repo.GetVersions(ctx, c.ID)
		if err != nil {
			continue
		}
		m.TotalVersions += int64(len(versions))
	}

	type metricsProvider interface {
		GetMetrics(ctx context.Context) (*Metrics, error)
	}
	if mp, ok := h.repo.(metricsProvider); ok {
		dbMetrics, err := mp.GetMetrics(ctx)
		if err == nil {
			m.TotalTraces = dbMetrics.TotalTraces
			m.TotalFeedback = dbMetrics.TotalFeedback
			m.TotalEvaluations = dbMetrics.TotalEvaluations
			m.AvgConfidence = dbMetrics.AvgConfidence
		}
	}

	writeJSON(w, http.StatusOK, m)
}

// --- Helpers ---

func (h *HTTPHandler) getCapsuleCount(ctx context.Context) (int, error) {
	capsules, err := h.repo.ListCapsules(ctx, "", "", "", 1000000)
	if err != nil {
		return 0, err
	}
	return len(capsules), nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": "error", "message": msg})
}
