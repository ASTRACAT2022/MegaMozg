package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mezamozg/meza-core/internal/domain"
)

type GeminiPlannerConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

type GeminiPlanner struct {
	client   *http.Client
	apiKey   string
	model    string
	baseURL  string
	fallback Planner
}

func NewGeminiPlanner(cfg GeminiPlannerConfig, fallback Planner) *GeminiPlanner {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	model := cfg.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}

	return &GeminiPlanner{
		client: &http.Client{
			Timeout: timeout,
		},
		apiKey:   cfg.APIKey,
		model:    model,
		baseURL:  baseURL,
		fallback: fallback,
	}
}

func (p *GeminiPlanner) Interpret(prompt string) domain.AIPlannedOperation {
	if strings.TrimSpace(p.apiKey) == "" {
		return withProvider(p.fallback.Interpret(prompt), "stub")
	}

	plan, err := p.plan(prompt)
	if err != nil {
		fallback := p.fallback.Interpret(prompt)
		fallback.Provider = "stub-fallback"
		fallback.Summary = fmt.Sprintf("%s Fallback planner used because Gemini was unavailable.", fallback.Summary)
		return fallback
	}

	return plan
}

func (p *GeminiPlanner) plan(prompt string) (domain.AIPlannedOperation, error) {
	requestBody := geminiRequest{
		SystemInstruction: geminiContent{
			Parts: []geminiPart{{
				Text: "You are the MezaMozg AI planner. Produce only a safe typed job plan. Never emit shell commands. Prefer rolling strategy for privileged operations. Set requires_review=true for any privileged or potentially disruptive change.",
			}},
		},
		Contents: []geminiContent{{
			Role: "user",
			Parts: []geminiPart{{
				Text: prompt,
			}},
		}},
		Tools: []geminiTool{{
			FunctionDeclarations: []geminiFunctionDeclaration{plannerFunctionDeclaration()},
		}},
		ToolConfig: geminiToolConfig{
			FunctionCallingConfig: geminiFunctionCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: []string{"plan_typed_job"},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature: 0.2,
		},
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return domain.AIPlannedOperation{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return domain.AIPlannedOperation{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return domain.AIPlannedOperation{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.AIPlannedOperation{}, err
	}

	if resp.StatusCode >= 300 {
		return domain.AIPlannedOperation{}, fmt.Errorf("gemini returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return domain.AIPlannedOperation{}, err
	}

	args, ok := extractGeminiFunctionArgs(parsed, "plan_typed_job")
	if !ok {
		return domain.AIPlannedOperation{}, fmt.Errorf("gemini response did not include plan_typed_job function call")
	}

	plan := domain.AIPlannedOperation{
		Prompt:         prompt,
		Provider:       "gemini",
		Intent:         stringArg(args, "intent", "inspect"),
		RequiresReview: boolArg(args, "requires_review", true),
		Summary:        stringArg(args, "summary", "Gemini produced a plan."),
		JobPreview: domain.JobCreateInput{
			Type:           nestedStringArg(args, "job_preview", "type", "manual_review"),
			TargetSelector: nestedStringArg(args, "job_preview", "target_selector", "fleet:all"),
			Strategy:       nestedStringArg(args, "job_preview", "strategy", "all-at-once"),
			Payload:        nestedMapArg(args, "job_preview", "payload"),
			CreatedBy:      "ai-copilot",
			Summary:        nestedStringArg(args, "job_preview", "summary", ""),
		},
	}

	if plan.JobPreview.Payload == nil {
		plan.JobPreview.Payload = map[string]any{}
	}

	return plan, nil
}

func plannerFunctionDeclaration() geminiFunctionDeclaration {
	return geminiFunctionDeclaration{
		Name:        "plan_typed_job",
		Description: "Plan a safe MezaMozg typed job from natural language operator intent.",
		Parameters: geminiSchema{
			Type: "OBJECT",
			Properties: map[string]geminiSchema{
				"intent": {
					Type: "STRING",
				},
				"requires_review": {
					Type: "BOOLEAN",
				},
				"summary": {
					Type: "STRING",
				},
				"job_preview": {
					Type: "OBJECT",
					Properties: map[string]geminiSchema{
						"type": {
							Type: "STRING",
							Enum: []string{"update_docker", "package_refresh", "service_restart", "manual_review"},
						},
						"target_selector": {
							Type: "STRING",
						},
						"strategy": {
							Type: "STRING",
						},
						"summary": {
							Type: "STRING",
						},
						"payload": {
							Type: "OBJECT",
						},
					},
					Required: []string{"type", "target_selector", "strategy"},
				},
			},
			Required: []string{"intent", "requires_review", "summary", "job_preview"},
		},
	}
}

func withProvider(plan domain.AIPlannedOperation, provider string) domain.AIPlannedOperation {
	plan.Provider = provider
	return plan
}

func extractGeminiFunctionArgs(resp geminiResponse, name string) (map[string]any, bool) {
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil || part.FunctionCall.Name != name {
				continue
			}
			return part.FunctionCall.Args, true
		}
	}

	return nil, false
}

func stringArg(values map[string]any, key string, fallback string) string {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	if asString, ok := value.(string); ok && strings.TrimSpace(asString) != "" {
		return asString
	}

	return fallback
}

func boolArg(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	if asBool, ok := value.(bool); ok {
		return asBool
	}

	return fallback
}

func nestedStringArg(values map[string]any, outerKey string, innerKey string, fallback string) string {
	outer, ok := values[outerKey].(map[string]any)
	if !ok {
		return fallback
	}

	return stringArg(outer, innerKey, fallback)
}

func nestedMapArg(values map[string]any, outerKey string, innerKey string) map[string]any {
	outer, ok := values[outerKey].(map[string]any)
	if !ok {
		return nil
	}

	payload, ok := outer[innerKey].(map[string]any)
	if !ok {
		return nil
	}

	return payload
}

type geminiRequest struct {
	SystemInstruction geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	Tools             []geminiTool           `json:"tools,omitempty"`
	ToolConfig        geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string              `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Parameters  geminiSchema `json:"parameters,omitempty"`
}

type geminiSchema struct {
	Type        string                  `json:"type,omitempty"`
	Description string                  `json:"description,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
	Properties  map[string]geminiSchema `json:"properties,omitempty"`
	Items       *geminiSchema           `json:"items,omitempty"`
	Required    []string                `json:"required,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig geminiFunctionCallingConfig `json:"functionCallingConfig"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type geminiFunctionCall struct {
	Name string         `json:"name,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}
