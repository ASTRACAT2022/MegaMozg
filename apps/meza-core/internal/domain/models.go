package domain

import "time"

type Node struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Region     string      `json:"region"`
	Tags       []string    `json:"tags"`
	Status     string      `json:"status"`
	Metrics    NodeMetrics `json:"metrics"`
	LastSeenAt time.Time   `json:"last_seen_at"`
	CreatedAt  time.Time   `json:"created_at"`
}

type NodeMetrics struct {
	CPUPercent     int `json:"cpu_percent"`
	RAMPercent     int `json:"ram_percent"`
	DiskPercent    int `json:"disk_percent"`
	NetworkKbps    int `json:"network_kbps"`
	LoadAverage    int `json:"load_average"`
	ProcessesCount int `json:"processes_count"`
}

type Job struct {
	ID               string              `json:"id"`
	Type             string              `json:"type"`
	TargetSelector   string              `json:"target_selector"`
	Strategy         string              `json:"strategy"`
	Status           string              `json:"status"`
	Payload          map[string]any      `json:"payload"`
	CreatedAt        time.Time           `json:"created_at"`
	CreatedBy        string              `json:"created_by"`
	RequiresApproval bool                `json:"requires_approval"`
	ApprovedBy       string              `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time          `json:"approved_at,omitempty"`
	StartedAt        *time.Time          `json:"started_at,omitempty"`
	CompletedAt      *time.Time          `json:"completed_at,omitempty"`
	Summary          string              `json:"summary"`
	MatchedNodes     []string            `json:"matched_nodes"`
	Rollout          RolloutProgress     `json:"rollout"`
	Plan             *AIPlannedOperation `json:"plan,omitempty"`
}

type RolloutProgress struct {
	Mode              string         `json:"mode"`
	CurrentBatchIndex int            `json:"current_batch_index"`
	CurrentBatchLabel string         `json:"current_batch_label"`
	CompletedNodes    int            `json:"completed_nodes"`
	FailedNodes       int            `json:"failed_nodes"`
	TotalNodes        int            `json:"total_nodes"`
	Batches           []RolloutBatch `json:"batches"`
}

type RolloutBatch struct {
	Label          string    `json:"label"`
	TargetedNodes  []string  `json:"targeted_nodes"`
	Status         string    `json:"status"`
	CompletedNodes int       `json:"completed_nodes"`
	FailedNodes    int       `json:"failed_nodes"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AuditEvent struct {
	ID           string         `json:"id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Message      string         `json:"message"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

type DashboardSummary struct {
	TotalNodes        int     `json:"total_nodes"`
	OnlineNodes       int     `json:"online_nodes"`
	DegradedNodes     int     `json:"degraded_nodes"`
	RunningJobs       int     `json:"running_jobs"`
	AwaitingApprovals int     `json:"awaiting_approvals"`
	AuditEvents       int     `json:"audit_events"`
	AverageCPU        float64 `json:"average_cpu"`
	AverageRAM        float64 `json:"average_ram"`
	AverageDisk       float64 `json:"average_disk"`
}

type AIInterpretRequest struct {
	Prompt string `json:"prompt"`
}

type AIPlannedOperation struct {
	Prompt         string         `json:"prompt"`
	Provider       string         `json:"provider"`
	Intent         string         `json:"intent"`
	RequiresReview bool           `json:"requires_review"`
	Summary        string         `json:"summary"`
	JobPreview     JobCreateInput `json:"job_preview"`
}

type NodeRegisterInput struct {
	Name   string   `json:"name"`
	Region string   `json:"region"`
	Tags   []string `json:"tags"`
}

type NodeHeartbeatInput struct {
	Name    string      `json:"name"`
	Region  string      `json:"region"`
	Tags    []string    `json:"tags"`
	Status  string      `json:"status"`
	Metrics NodeMetrics `json:"metrics"`
}

type JobCreateInput struct {
	Type           string         `json:"type"`
	TargetSelector string         `json:"target_selector"`
	Strategy       string         `json:"strategy"`
	Payload        map[string]any `json:"payload"`
	CreatedBy      string         `json:"created_by"`
	Summary        string         `json:"summary"`
}

type JobActionInput struct {
	Actor string `json:"actor"`
}
