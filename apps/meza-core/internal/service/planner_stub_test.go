package service

import "testing"

func TestStubPlannerBuildsBind9UpdateForFleet(t *testing.T) {
	planner := NewStubPlanner()

	plan := planner.Interpret("выполни обновления bind9 на всех серверах")
	if plan.Intent != "update_bind9" {
		t.Fatalf("expected update_bind9 intent, got %q", plan.Intent)
	}
	if plan.JobPreview.Type != "shell_command" {
		t.Fatalf("expected shell_command type, got %q", plan.JobPreview.Type)
	}
	if plan.JobPreview.TargetSelector != "fleet:all" {
		t.Fatalf("expected fleet:all target, got %q", plan.JobPreview.TargetSelector)
	}
	command, ok := plan.JobPreview.Payload["command"].(string)
	if !ok || command == "" {
		t.Fatalf("expected non-empty command payload")
	}
}
