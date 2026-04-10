package domain

import "time"

const (
	DefaultAIProvider    = "stub"
	DefaultGeminiModel   = "gemini-2.5-flash"
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

type Node struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	IPAddress   string      `json:"ip_address"`
	Region      string      `json:"region"`
	Tags        []string    `json:"tags"`
	Status      string      `json:"status"`
	Metrics     NodeMetrics `json:"metrics"`
	LastSeenAt  time.Time   `json:"last_seen_at"`
	CreatedAt   time.Time   `json:"created_at"`
}

type NodeMetrics struct {
	CPUPercent     int `json:"cpu_percent"`
	RAMPercent     int `json:"ram_percent"`
	DiskPercent    int `json:"disk_percent"`
	NetworkKbps    int `json:"network_kbps"`
	LoadAverage    int `json:"load_average"`
	ProcessesCount int `json:"processes_count"`
}

type NodeMetricSample struct {
	NodeName   string      `json:"node_name"`
	Timestamp  time.Time   `json:"timestamp"`
	Status     string      `json:"status"`
	Region     string      `json:"region"`
	Tags       []string    `json:"tags"`
	Metrics    NodeMetrics `json:"metrics"`
	LastSeenAt time.Time   `json:"last_seen_at"`
}

type NodeFreeScore struct {
	NodeName       string    `json:"node_name"`
	DisplayName    string    `json:"display_name"`
	Region         string    `json:"region"`
	Status         string    `json:"status"`
	Samples        int       `json:"samples"`
	WindowHours    int       `json:"window_hours"`
	AverageCPU     float64   `json:"average_cpu"`
	AverageRAM     float64   `json:"average_ram"`
	AverageDisk    float64   `json:"average_disk"`
	AverageLoad    float64   `json:"average_load"`
	AverageNetKbps float64   `json:"average_net_kbps"`
	FreeScore      float64   `json:"free_score"`
	LastSeenAt     time.Time `json:"last_seen_at"`
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

type AIConfig struct {
	Provider        string     `json:"provider"`
	GeminiModel     string     `json:"gemini_model"`
	GeminiBaseURL   string     `json:"gemini_base_url"`
	HasGeminiAPIKey bool       `json:"has_gemini_api_key"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type AIConfigUpdateInput struct {
	Provider          string `json:"provider"`
	GeminiAPIKey      string `json:"gemini_api_key"`
	ClearGeminiAPIKey bool   `json:"clear_gemini_api_key"`
	GeminiModel       string `json:"gemini_model"`
	GeminiBaseURL     string `json:"gemini_base_url"`
}

type AISettings struct {
	Provider      string     `json:"provider"`
	GeminiAPIKey  string     `json:"gemini_api_key"`
	GeminiModel   string     `json:"gemini_model"`
	GeminiBaseURL string     `json:"gemini_base_url"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type AIPlannedOperation struct {
	Prompt         string         `json:"prompt"`
	Provider       string         `json:"provider"`
	Intent         string         `json:"intent"`
	RequiresReview bool           `json:"requires_review"`
	Summary        string         `json:"summary"`
	JobPreview     JobCreateInput `json:"job_preview"`
}

type AIChatResponse struct {
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

type NodeRegisterInput struct {
	Name      string   `json:"name"`
	Region    string   `json:"region"`
	Tags      []string `json:"tags"`
	IPAddress string   `json:"ip_address"`
}

type NodeHeartbeatInput struct {
	Name      string      `json:"name"`
	Region    string      `json:"region"`
	Tags      []string    `json:"tags"`
	Status    string      `json:"status"`
	IPAddress string      `json:"ip_address"`
	Metrics   NodeMetrics `json:"metrics"`
}

type NodeUpdateInput struct {
	DisplayName string `json:"display_name"`
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
