package service

import (
	"testing"

	"github.com/mezamozg/meza-core/internal/domain"
)

func TestRuntimePlannerReturnsMissingKeyPlanWhenGeminiWithoutKey(t *testing.T) {
	planner := NewRuntimePlanner(RuntimePlannerConfig{
		Provider: "gemini",
		Fallback: NewStubPlanner(),
	})

	plan := planner.Interpret("обнови docker на ноде argentina-17")
	if plan.Provider != "gemini-missing-key" {
		t.Fatalf("expected provider gemini-missing-key, got %q", plan.Provider)
	}
}

func TestRuntimePlannerCurrentConfigReflectsAppliedSettings(t *testing.T) {
	planner := NewRuntimePlanner(RuntimePlannerConfig{
		Fallback: NewStubPlanner(),
	})

	planner.ApplySettings(domain.AISettings{
		Provider:      "gemini",
		GeminiAPIKey:  "key",
		GeminiModel:   "gemini-2.5-flash",
		GeminiBaseURL: "https://generativelanguage.googleapis.com/v1beta",
	})

	cfg := planner.CurrentConfig()
	if cfg.Provider != "gemini" {
		t.Fatalf("expected provider gemini, got %q", cfg.Provider)
	}
	if !cfg.HasGeminiAPIKey {
		t.Fatalf("expected has_gemini_api_key=true")
	}
}
