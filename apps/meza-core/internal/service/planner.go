package service

import "github.com/mezamozg/meza-core/internal/domain"

type Planner interface {
	Interpret(prompt string) domain.AIPlannedOperation
}

type FallbackPlanner struct {
	primary  Planner
	fallback Planner
}

func NewFallbackPlanner(primary Planner, fallback Planner) *FallbackPlanner {
	return &FallbackPlanner{
		primary:  primary,
		fallback: fallback,
	}
}

func (p *FallbackPlanner) Interpret(prompt string) domain.AIPlannedOperation {
	if p.primary == nil {
		return p.fallback.Interpret(prompt)
	}

	plan := p.primary.Interpret(prompt)
	if plan.Intent == "" {
		return p.fallback.Interpret(prompt)
	}

	return plan
}
