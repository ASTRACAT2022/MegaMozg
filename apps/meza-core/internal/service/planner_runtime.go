package service

import (
	"fmt"
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

func (p *RuntimePlanner) Chat(prompt string) domain.AIChatResponse {
	p.mu.RLock()
	provider := p.provider
	apiKey := p.geminiAPIKey
	model := p.geminiModel
	baseURL := p.geminiBaseURL
	timeout := p.timeout
	fallback := p.fallback
	p.mu.RUnlock()

	if provider == "gemini" && strings.TrimSpace(apiKey) != "" {
		geminiPlanner := NewGeminiPlanner(GeminiPlannerConfig{
			APIKey:  apiKey,
			Model:   model,
			BaseURL: baseURL,
			Timeout: timeout,
		}, fallback)
		text, err := geminiPlanner.Chat(prompt)
		if err == nil && strings.TrimSpace(text) != "" {
			return domain.AIChatResponse{
				Provider: "gemini",
				Message:  strings.TrimSpace(text),
			}
		}

		if err != nil {
			return domain.AIChatResponse{
				Provider: "gemini-error",
				Message:  fmt.Sprintf("Gemini временно недоступен: %v. Проверь API key, интернет на хабе и Gemini base URL.", err),
			}
		}
	}

	return domain.AIChatResponse{
		Provider: "stub-chat",
		Message:  fallbackChatReply(prompt),
	}
}

func fallbackChatReply(prompt string) string {
	normalized := strings.ToLower(strings.TrimSpace(prompt))

	switch {
	case strings.Contains(normalized, "кто ты"):
		return "Я AI-агент MezaMozg. Помогаю запускать операции на нодах и контролировать rollout без ручной рутины."
	case strings.Contains(normalized, "что ты можешь"), strings.Contains(normalized, "можешь"):
		return "Могу подготовить и запускать задачи: обновление Docker, обновление пакетов, рестарт сервисов, поэтапный rollout и проверка статусов нод."
	case strings.Contains(normalized, "привет"), strings.Contains(normalized, "hello"), strings.Contains(normalized, "hi"):
		return "Привет. Напиши, что сделать на инфраструктуре, например: «обнови docker на ноде astra-1»."
	default:
		return fmt.Sprintf("Понял запрос: «%s». Если нужна операция, укажи действие и цель: нода/тег/fleet.", strings.TrimSpace(prompt))
	}
}

func normalizeAISettings(settings domain.AISettings) domain.AISettings {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	hasGeminiKey := strings.TrimSpace(settings.GeminiAPIKey) != ""
	if provider != "gemini" {
		if hasGeminiKey {
			provider = "gemini"
		} else {
			provider = domain.DefaultAIProvider
		}
	}
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
