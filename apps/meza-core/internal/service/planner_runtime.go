package service

import (
	"strings"
	"sync"
	"time"

	"github.com/mezamozg/meza-core/internal/domain"
)

type RuntimePlannerConfig struct {
	Provider      string
	GeminiAPIKey  string
	GeminiModel   string
	GeminiBaseURL string
	Timeout       time.Duration
	Fallback      Planner
}

type RuntimePlanner struct {
	mu            sync.RWMutex
	provider      string
	geminiAPIKey  string
	geminiModel   string
	geminiBaseURL string
	updatedAt     *time.Time
	timeout       time.Duration
	fallback      Planner
}

func NewRuntimePlanner(cfg RuntimePlannerConfig) *RuntimePlanner {
	if cfg.Fallback == nil {
		cfg.Fallback = NewStubPlanner()
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	planner := &RuntimePlanner{
		timeout:  timeout,
		fallback: cfg.Fallback,
	}

	planner.ApplySettings(domain.AISettings{
		Provider:      cfg.Provider,
		GeminiAPIKey:  cfg.GeminiAPIKey,
		GeminiModel:   cfg.GeminiModel,
		GeminiBaseURL: cfg.GeminiBaseURL,
	})

	return planner
}

func (p *RuntimePlanner) Interpret(prompt string) domain.AIPlannedOperation {
	p.mu.RLock()
	provider := p.provider
	apiKey := p.geminiAPIKey
	model := p.geminiModel
	baseURL := p.geminiBaseURL
	timeout := p.timeout
	fallback := p.fallback
	p.mu.RUnlock()

	if provider != "gemini" {
		return fallback.Interpret(prompt)
	}

	if strings.TrimSpace(apiKey) == "" {
		return domain.AIPlannedOperation{
			Prompt:         prompt,
			Provider:       "gemini-missing-key",
			Intent:         "missing_key",
			RequiresReview: true,
			Summary:        "Gemini API key не задан. Добавьте ключ в настройках AI в панели.",
			JobPreview: domain.JobCreateInput{
				Type:           "manual_review",
				TargetSelector: "fleet:all",
				Strategy:       "all-at-once",
				Payload: map[string]any{
					"reason": "gemini_api_key_missing",
				},
				CreatedBy: "ai-copilot",
			},
		}
	}

	geminiPlanner := NewGeminiPlanner(GeminiPlannerConfig{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
		Timeout: timeout,
	}, fallback)

	return geminiPlanner.Interpret(prompt)
}

func (p *RuntimePlanner) ApplySettings(settings domain.AISettings) {
	normalized := normalizeAISettings(settings)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.provider = normalized.Provider
	p.geminiAPIKey = strings.TrimSpace(normalized.GeminiAPIKey)
	p.geminiModel = normalized.GeminiModel
	p.geminiBaseURL = normalized.GeminiBaseURL
	p.updatedAt = normalized.UpdatedAt
}

func (p *RuntimePlanner) CurrentConfig() domain.AIConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return domain.AIConfig{
		Provider:        p.provider,
		GeminiModel:     p.geminiModel,
		GeminiBaseURL:   p.geminiBaseURL,
		HasGeminiAPIKey: strings.TrimSpace(p.geminiAPIKey) != "",
		UpdatedAt:       p.updatedAt,
	}
}

func normalizeAISettings(settings domain.AISettings) domain.AISettings {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if provider != "gemini" {
		provider = domain.DefaultAIProvider
	}

	model := strings.TrimSpace(settings.GeminiModel)
	if model == "" {
		model = domain.DefaultGeminiModel
	}

	baseURL := strings.TrimRight(strings.TrimSpace(settings.GeminiBaseURL), "/")
	if baseURL == "" {
		baseURL = domain.DefaultGeminiBaseURL
	}

	updatedAt := settings.UpdatedAt
	if updatedAt == nil {
		now := time.Now().UTC()
		updatedAt = &now
	}

	return domain.AISettings{
		Provider:      provider,
		GeminiAPIKey:  strings.TrimSpace(settings.GeminiAPIKey),
		GeminiModel:   model,
		GeminiBaseURL: baseURL,
		UpdatedAt:     updatedAt,
	}
}
