package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mezamozg/meza-core/internal/domain"
	"github.com/mezamozg/meza-core/internal/service"
)

type Server struct {
	store   *service.MemoryStore
	planner service.Planner
	auth    AuthConfig
	mux     *http.ServeMux
}

func NewServer(store *service.MemoryStore, planner service.Planner, auth AuthConfig) *Server {
	s := &Server{
		store:   store,
		planner: planner,
		auth:    auth,
		mux:     http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /api/v1/dashboard", s.requireAuth(scopeOperator, s.handleDashboard))
	s.mux.HandleFunc("GET /api/v1/nodes", s.requireAuth(scopeOperator, s.handleListNodes))
	s.mux.HandleFunc("POST /api/v1/nodes/register", s.requireAuth(scopeBootstrap, s.handleRegisterNode))
	s.mux.HandleFunc("POST /api/v1/nodes/heartbeat", s.requireAuthAny([]authScope{scopeNode, scopeBootstrap}, s.handleHeartbeatNode))
	s.mux.HandleFunc("GET /api/v1/jobs", s.requireAuth(scopeOperator, s.handleListJobs))
	s.mux.HandleFunc("GET /api/v1/jobs/{id}", s.requireAuth(scopeOperator, s.handleGetJob))
	s.mux.HandleFunc("POST /api/v1/jobs", s.requireAuth(scopeOperator, s.handleCreateJob))
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/approve", s.requireAuth(scopeOperator, s.handleApproveJob))
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/start", s.requireAuth(scopeOperator, s.handleStartJob))
	s.mux.HandleFunc("DELETE /api/v1/jobs/{id}", s.requireAuth(scopeOperator, s.handleDeleteJob))
	s.mux.HandleFunc("POST /api/v1/terminal/stream", s.requireAuth(scopeOperator, s.handleTerminalStream))
	s.mux.HandleFunc("GET /api/v1/audit", s.requireAuth(scopeOperator, s.handleListAuditEvents))
	s.mux.HandleFunc("GET /api/v1/ai/config", s.requireAuth(scopeOperator, s.handleAIConfig))
	s.mux.HandleFunc("POST /api/v1/ai/config", s.requireAuth(scopeOperator, s.handleAIConfigUpdate))
	s.mux.HandleFunc("POST /api/v1/ai/interpret", s.requireAuth(scopeOperator, s.handleAIInterpret))
	s.mux.HandleFunc("POST /api/v1/ai/plan-and-create", s.requireAuth(scopeOperator, s.handleAIPlanAndCreate))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "meza-core",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.DashboardSummary())
}

func (s *Server) handleListNodes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.store.ListNodes(),
	})
}

func (s *Server) handleRegisterNode(w http.ResponseWriter, r *http.Request) {
	var input domain.NodeRegisterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if input.Name == "" || input.Region == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and region are required"})
		return
	}

	node := s.store.RegisterNode(input)
	writeJSON(w, http.StatusCreated, node)
}

func (s *Server) handleHeartbeatNode(w http.ResponseWriter, r *http.Request) {
	var input domain.NodeHeartbeatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if input.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	node := s.store.HeartbeatNode(input)
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.store.ListJobs(),
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var input domain.JobCreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if input.Type == "" || input.TargetSelector == "" || input.Strategy == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type, target_selector, and strategy are required"})
		return
	}

	if input.CreatedBy == "" {
		input.CreatedBy = "operator"
	}

	job := s.store.CreateJob(input, nil)
	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) handleApproveJob(w http.ResponseWriter, r *http.Request) {
	var input domain.JobActionInput
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.Actor == "" {
		input.Actor = "operator"
	}

	job, err := s.store.ApproveJob(r.PathValue("id"), input.Actor)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleStartJob(w http.ResponseWriter, r *http.Request) {
	var input domain.JobActionInput
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.Actor == "" {
		input.Actor = "operator"
	}

	job, err := s.store.StartJob(r.PathValue("id"), input.Actor)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	var input domain.JobActionInput
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.Actor == "" {
		input.Actor = "operator"
	}

	job, err := s.store.DeleteJob(r.PathValue("id"), input.Actor)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleListAuditEvents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.store.ListAuditEvents(),
	})
}

func (s *Server) handleAIConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.currentAIConfig())
}

func (s *Server) handleAIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var input domain.AIConfigUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	settings := s.store.UpdateAISettings(input)
	if planner, ok := s.planner.(service.AIConfigurablePlanner); ok {
		planner.ApplySettings(settings)
		writeJSON(w, http.StatusOK, planner.CurrentConfig())
		return
	}

	writeJSON(w, http.StatusOK, domain.AIConfig{
		Provider:        settings.Provider,
		GeminiModel:     settings.GeminiModel,
		GeminiBaseURL:   settings.GeminiBaseURL,
		HasGeminiAPIKey: settings.GeminiAPIKey != "",
		UpdatedAt:       settings.UpdatedAt,
	})
}

func (s *Server) handleAIInterpret(w http.ResponseWriter, r *http.Request) {
	var input domain.AIInterpretRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if input.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}

	plan := s.planner.Interpret(input.Prompt)
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleAIPlanAndCreate(w http.ResponseWriter, r *http.Request) {
	var input domain.AIInterpretRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if input.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}

	plan := s.planner.Interpret(input.Prompt)
	if plan.Provider == "gemini-missing-key" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "Gemini API key is missing. Configure it in /api/v1/ai/config first.",
			"plan":  plan,
		})
		return
	}

	job := s.store.CreateJob(plan.JobPreview, &plan)

	writeJSON(w, http.StatusCreated, map[string]any{
		"plan": plan,
		"job":  job,
	})
}

func (s *Server) currentAIConfig() domain.AIConfig {
	if planner, ok := s.planner.(service.AIConfigurablePlanner); ok {
		return planner.CurrentConfig()
	}

	settings := s.store.GetAISettings()
	return domain.AIConfig{
		Provider:        settings.Provider,
		GeminiModel:     settings.GeminiModel,
		GeminiBaseURL:   settings.GeminiBaseURL,
		HasGeminiAPIKey: settings.GeminiAPIKey != "",
		UpdatedAt:       settings.UpdatedAt,
	}
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrJobNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrJobNeedsApproval), errors.Is(err, service.ErrJobCannotApprove), errors.Is(err, service.ErrJobCannotStart):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}
