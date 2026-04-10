package service

import (
	"strings"

	"github.com/mezamozg/meza-core/internal/domain"
)

type StubPlanner struct{}

func NewStubPlanner() *StubPlanner {
	return &StubPlanner{}
}

func (p *StubPlanner) Interpret(prompt string) domain.AIPlannedOperation {
	normalized := strings.ToLower(strings.TrimSpace(prompt))

	plan := domain.AIPlannedOperation{
		Prompt:         prompt,
		Provider:       "stub",
		Intent:         "inspect",
		RequiresReview: true,
		Summary:        "AI planner could not confidently classify the request. Manual review is required.",
		JobPreview: domain.JobCreateInput{
			Type:           "manual_review",
			TargetSelector: "fleet:all",
			Strategy:       "all-at-once",
			Payload: map[string]any{
				"prompt": prompt,
			},
			CreatedBy: "ai-copilot",
		},
	}

	switch {
	case strings.Contains(normalized, "docker") && (strings.Contains(normalized, "обнов") || strings.Contains(normalized, "update")):
		plan.Intent = "update_docker"
		plan.RequiresReview = true
		plan.Summary = "Planned a Docker update job. Because this is a privileged package operation, review and approval are required before execution."
		plan.JobPreview = domain.JobCreateInput{
			Type:           "update_docker",
			TargetSelector: extractTarget(normalized),
			Strategy:       "rolling:10,25,50,100",
			Payload: map[string]any{
				"channel": "stable",
			},
			CreatedBy: "ai-copilot",
		}
	case strings.Contains(normalized, "apt update") || strings.Contains(normalized, "обнови пакеты"):
		plan.Intent = "package_refresh"
		plan.RequiresReview = false
		plan.Summary = "Planned a package metadata refresh job."
		plan.JobPreview = domain.JobCreateInput{
			Type:           "package_refresh",
			TargetSelector: extractTarget(normalized),
			Strategy:       "rolling:25,50,100",
			Payload: map[string]any{
				"manager": "apt",
			},
			CreatedBy: "ai-copilot",
		}
	case strings.Contains(normalized, "restart") || strings.Contains(normalized, "перезапусти"):
		plan.Intent = "service_restart"
		plan.RequiresReview = true
		plan.Summary = "Planned a service restart job. Operator confirmation is recommended."
		plan.JobPreview = domain.JobCreateInput{
			Type:           "service_restart",
			TargetSelector: extractTarget(normalized),
			Strategy:       "rolling:10,25,50,100",
			Payload: map[string]any{
				"service": extractService(normalized),
			},
			CreatedBy: "ai-copilot",
		}
	}

	return plan
}

func extractTarget(prompt string) string {
	switch {
	case strings.Contains(prompt, "argentina-17"):
		return "node:argentina-17"
	case strings.Contains(prompt, "moscow"):
		return "tag:moscow"
	case strings.Contains(prompt, "staging"):
		return "tag:staging"
	default:
		return "fleet:all"
	}
}

func extractService(prompt string) string {
	switch {
	case strings.Contains(prompt, "docker"):
		return "docker"
	case strings.Contains(prompt, "nginx"):
		return "nginx"
	default:
		return "unknown"
	}
}
