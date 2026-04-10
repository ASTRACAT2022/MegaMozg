package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mezamozg/meza-core/internal/domain"
)

var (
	ErrJobNotFound      = errors.New("job not found")
	ErrJobNeedsApproval = errors.New("job requires approval before start")
	ErrJobCannotStart   = errors.New("job cannot be started from its current state")
	ErrJobCannotApprove = errors.New("job cannot be approved from its current state")
	ErrNodeNotFound     = errors.New("node not found")
)

const (
	metricHistoryRetention  = 72 * time.Hour
	metricSampleMinInterval = 5 * time.Minute
)

type MemoryStore struct {
	mu                 sync.RWMutex
	nodes              []domain.Node
	metricSamples      []domain.NodeMetricSample
	lastMetricSampleAt map[string]time.Time
	jobs               []domain.Job
	audits             []domain.AuditEvent
	aiSettings         domain.AISettings
	persistPath        string
}

func NewMemoryStore() *MemoryStore {
	store, err := NewMemoryStoreWithFile("")
	if err != nil {
		panic(err)
	}
	return store
}

type persistedState struct {
	Nodes         []domain.Node             `json:"nodes"`
	MetricSamples []domain.NodeMetricSample `json:"metric_samples"`
	Jobs          []domain.Job              `json:"jobs"`
	Audits        []domain.AuditEvent       `json:"audits"`
	AISettings    domain.AISettings         `json:"ai_settings"`
}

func NewMemoryStoreWithFile(persistPath string) (*MemoryStore, error) {
	if persistPath != "" {
		if state, err := loadState(persistPath); err == nil {
			state, changed := sanitizeSeededDemoState(state)
			dedupedNodes, removedByIP := deduplicateNodesByIP(state.Nodes)
			if removedByIP > 0 {
				state.Nodes = dedupedNodes
				changed = true
			}
			filteredSamples, removedSamples := pruneMetricSamples(state.MetricSamples, time.Now().UTC(), metricHistoryRetention)
			if removedSamples > 0 {
				state.MetricSamples = filteredSamples
				changed = true
			}
			if changed {
				if err := saveState(persistPath, state); err != nil {
					return nil, err
				}
			}

			return &MemoryStore{
				nodes:              state.Nodes,
				metricSamples:      state.MetricSamples,
				lastMetricSampleAt: buildLastMetricSampleIndex(state.MetricSamples),
				jobs:               state.Jobs,
				audits:             state.Audits,
				aiSettings:         normalizeAISettings(state.AISettings),
				persistPath:        persistPath,
			}, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	store := &MemoryStore{
		persistPath:        persistPath,
		nodes:              []domain.Node{},
		metricSamples:      []domain.NodeMetricSample{},
		lastMetricSampleAt: map[string]time.Time{},
		jobs:               []domain.Job{},
		audits:             []domain.AuditEvent{},
		aiSettings:         normalizeAISettings(domain.AISettings{}),
	}
	return store, nil
}

func (s *MemoryStore) ListNodes() []domain.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Node, len(s.nodes))
	copy(out, s.nodes)
	return out
}

func (s *MemoryStore) TopFreeNodes(window time.Duration, limit int) []domain.NodeFreeScore {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}
	if window <= 0 {
		window = 48 * time.Hour
	}

	cutoff := time.Now().UTC().Add(-window)

	type agg struct {
		count      int
		cpuSum     int
		ramSum     int
		diskSum    int
		loadSum    int
		networkSum int
		lastSeenAt time.Time
		status     string
		region     string
	}

	byNode := map[string]*agg{}
	nodeByName := map[string]domain.Node{}
	for _, node := range s.nodes {
		nodeByName[node.Name] = node
	}

	for _, sample := range s.metricSamples {
		if sample.Timestamp.Before(cutoff) {
			continue
		}

		entry := byNode[sample.NodeName]
		if entry == nil {
			entry = &agg{}
			byNode[sample.NodeName] = entry
		}

		entry.count++
		entry.cpuSum += sample.Metrics.CPUPercent
		entry.ramSum += sample.Metrics.RAMPercent
		entry.diskSum += sample.Metrics.DiskPercent
		entry.loadSum += sample.Metrics.LoadAverage
		entry.networkSum += sample.Metrics.NetworkKbps
		if sample.LastSeenAt.After(entry.lastSeenAt) {
			entry.lastSeenAt = sample.LastSeenAt
		}
		entry.status = sample.Status
		entry.region = sample.Region
	}

	out := make([]domain.NodeFreeScore, 0, len(byNode))
	for nodeName, aggregate := range byNode {
		if aggregate.count == 0 {
			continue
		}

		nodeMeta := nodeByName[nodeName]
		avgCPU := float64(aggregate.cpuSum) / float64(aggregate.count)
		avgRAM := float64(aggregate.ramSum) / float64(aggregate.count)
		avgDisk := float64(aggregate.diskSum) / float64(aggregate.count)
		avgLoad := float64(aggregate.loadSum) / float64(aggregate.count)
		avgNetwork := float64(aggregate.networkSum) / float64(aggregate.count)
		loadPenalty := avgLoad * 10
		if loadPenalty > 100 {
			loadPenalty = 100
		}
		weightedLoad := (avgCPU * 0.45) + (avgRAM * 0.35) + (avgDisk * 0.15) + (loadPenalty * 0.05)
		freeScore := 100 - weightedLoad
		if freeScore < 0 {
			freeScore = 0
		}

		out = append(out, domain.NodeFreeScore{
			NodeName:       nodeName,
			DisplayName:    nodeMeta.DisplayName,
			Region:         coalesce(nodeMeta.Region, aggregate.region),
			Status:         coalesce(nodeMeta.Status, aggregate.status),
			Samples:        aggregate.count,
			WindowHours:    int(window.Hours()),
			AverageCPU:     round1(avgCPU),
			AverageRAM:     round1(avgRAM),
			AverageDisk:    round1(avgDisk),
			AverageLoad:    round1(avgLoad),
			AverageNetKbps: round1(avgNetwork),
			FreeScore:      round1(freeScore),
			LastSeenAt:     aggregate.lastSeenAt,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].FreeScore == out[j].FreeScore {
			if out[i].AverageCPU == out[j].AverageCPU {
				return out[i].NodeName < out[j].NodeName
			}
			return out[i].AverageCPU < out[j].AverageCPU
		}
		return out[i].FreeScore > out[j].FreeScore
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *MemoryStore) RegisterNode(input domain.NodeRegisterInput) domain.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	normalizedIP := strings.TrimSpace(input.IPAddress)
	input.Name = strings.TrimSpace(input.Name)
	input.Region = strings.TrimSpace(input.Region)

	for idx := range s.nodes {
		if s.nodes[idx].Name != input.Name {
			continue
		}

		s.nodes[idx].Region = coalesce(input.Region, s.nodes[idx].Region)
		s.nodes[idx].Tags = fallbackTags(input.Tags, s.nodes[idx].Tags)
		s.nodes[idx].Status = "online"
		s.nodes[idx].IPAddress = coalesce(normalizedIP, s.nodes[idx].IPAddress)
		s.nodes[idx].LastSeenAt = now

		removed := s.deduplicateNodesByIPLocked("bootstrap-token")
		s.appendAuditLocked("bootstrap-token", "register", "node", s.nodes[idx].ID, "Node registered", map[string]any{
			"name":       s.nodes[idx].Name,
			"region":     s.nodes[idx].Region,
			"ipAddress":  s.nodes[idx].IPAddress,
			"duplicates": removed,
		})
		s.appendMetricSampleLocked(s.nodes[idx], now)
		s.saveLocked()
		return s.nodes[idx]
	}

	if normalizedIP != "" {
		for idx := range s.nodes {
			if strings.TrimSpace(s.nodes[idx].IPAddress) != normalizedIP {
				continue
			}

			s.nodes[idx].Name = coalesce(input.Name, s.nodes[idx].Name)
			s.nodes[idx].Region = coalesce(input.Region, s.nodes[idx].Region)
			s.nodes[idx].Tags = fallbackTags(input.Tags, s.nodes[idx].Tags)
			s.nodes[idx].Status = "online"
			s.nodes[idx].IPAddress = normalizedIP
			s.nodes[idx].LastSeenAt = now

			removed := s.deduplicateNodesByIPLocked("bootstrap-token")
			s.appendAuditLocked("bootstrap-token", "register", "node", s.nodes[idx].ID, "Node registered", map[string]any{
				"name":       s.nodes[idx].Name,
				"region":     s.nodes[idx].Region,
				"ipAddress":  s.nodes[idx].IPAddress,
				"duplicates": removed,
			})
			s.appendMetricSampleLocked(s.nodes[idx], now)
			s.saveLocked()
			return s.nodes[idx]
		}
	}

	node := domain.Node{
		ID:        s.nextNodeIDLocked(),
		Name:      input.Name,
		IPAddress: normalizedIP,
		Region:    input.Region,
		Tags:      input.Tags,
		Status:    "online",
		Metrics: domain.NodeMetrics{
			CPUPercent:     0,
			RAMPercent:     0,
			DiskPercent:    0,
			NetworkKbps:    0,
			LoadAverage:    0,
			ProcessesCount: 0,
		},
		LastSeenAt: now,
		CreatedAt:  now,
	}

	s.nodes = append(s.nodes, node)
	removed := s.deduplicateNodesByIPLocked("bootstrap-token")
	s.appendAuditLocked("bootstrap-token", "register", "node", node.ID, "Node registered", map[string]any{
		"name":       node.Name,
		"region":     node.Region,
		"ipAddress":  node.IPAddress,
		"duplicates": removed,
	})
	s.appendMetricSampleLocked(node, now)
	s.saveLocked()
	return node
}

func (s *MemoryStore) HeartbeatNode(input domain.NodeHeartbeatInput) domain.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	input.Name = strings.TrimSpace(input.Name)
	input.Region = strings.TrimSpace(input.Region)
	input.IPAddress = strings.TrimSpace(input.IPAddress)
	now := time.Now().UTC()
	for idx := range s.nodes {
		if s.nodes[idx].Name == input.Name {
			s.nodes[idx].Region = coalesce(input.Region, s.nodes[idx].Region)
			s.nodes[idx].Tags = fallbackTags(input.Tags, s.nodes[idx].Tags)
			s.nodes[idx].Status = coalesce(input.Status, s.nodes[idx].Status)
			s.nodes[idx].IPAddress = coalesce(input.IPAddress, s.nodes[idx].IPAddress)
			s.nodes[idx].Metrics = input.Metrics
			s.nodes[idx].LastSeenAt = now

			s.appendAuditLocked("meza-node", "heartbeat", "node", s.nodes[idx].ID, "Node heartbeat received", map[string]any{
				"name":      s.nodes[idx].Name,
				"status":    s.nodes[idx].Status,
				"ipAddress": s.nodes[idx].IPAddress,
			})
			s.deduplicateNodesByIPLocked("meza-node")
			s.appendMetricSampleLocked(s.nodes[idx], now)
			s.saveLocked()
			return s.nodes[idx]
		}
	}

	if input.IPAddress != "" {
		for idx := range s.nodes {
			if strings.TrimSpace(s.nodes[idx].IPAddress) != input.IPAddress {
				continue
			}

			s.nodes[idx].Name = coalesce(input.Name, s.nodes[idx].Name)
			s.nodes[idx].Region = coalesce(input.Region, s.nodes[idx].Region)
			s.nodes[idx].Tags = fallbackTags(input.Tags, s.nodes[idx].Tags)
			s.nodes[idx].Status = coalesce(input.Status, s.nodes[idx].Status)
			s.nodes[idx].IPAddress = input.IPAddress
			s.nodes[idx].Metrics = input.Metrics
			s.nodes[idx].LastSeenAt = now

			s.appendAuditLocked("meza-node", "heartbeat", "node", s.nodes[idx].ID, "Node heartbeat received", map[string]any{
				"name":      s.nodes[idx].Name,
				"status":    s.nodes[idx].Status,
				"ipAddress": s.nodes[idx].IPAddress,
			})
			s.deduplicateNodesByIPLocked("meza-node")
			s.appendMetricSampleLocked(s.nodes[idx], now)
			s.saveLocked()
			return s.nodes[idx]
		}
	}

	node := domain.Node{
		ID:         s.nextNodeIDLocked(),
		Name:       input.Name,
		IPAddress:  input.IPAddress,
		Region:     coalesce(input.Region, "unknown-region"),
		Tags:       input.Tags,
		Status:     coalesce(input.Status, "online"),
		Metrics:    input.Metrics,
		LastSeenAt: now,
		CreatedAt:  now,
	}
	s.nodes = append(s.nodes, node)
	s.appendAuditLocked("meza-node", "heartbeat-register", "node", node.ID, "Node heartbeat created a new inventory record", map[string]any{
		"name":      node.Name,
		"ipAddress": node.IPAddress,
	})
	s.deduplicateNodesByIPLocked("meza-node")
	s.appendMetricSampleLocked(node, now)
	s.saveLocked()
	return node
}

func (s *MemoryStore) UpdateNode(id string, input domain.NodeUpdateInput, actor string) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx := range s.nodes {
		if s.nodes[idx].ID != id {
			continue
		}

		s.nodes[idx].DisplayName = strings.TrimSpace(input.DisplayName)
		s.appendAuditLocked(actor, "update", "node", id, "Node metadata updated", map[string]any{
			"displayName": s.nodes[idx].DisplayName,
		})
		s.saveLocked()
		return s.nodes[idx], nil
	}

	return domain.Node{}, ErrNodeNotFound
}

func (s *MemoryStore) ListJobs() []domain.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Job, len(s.jobs))
	copy(out, s.jobs)
	return out
}

func (s *MemoryStore) GetJob(id string) (domain.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, job := range s.jobs {
		if job.ID == id {
			return job, nil
		}
	}

	return domain.Job{}, ErrJobNotFound
}

func (s *MemoryStore) CreateJob(input domain.JobCreateInput, plan *domain.AIPlannedOperation) domain.Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	approval := requiresApproval(input.Type)
	status := "approved"
	if approval {
		status = "awaiting_approval"
	}

	if input.CreatedBy == "" {
		input.CreatedBy = "operator"
	}

	matched := s.resolveTargetNamesLocked(input.TargetSelector)
	job := domain.Job{
		ID:               s.nextJobIDLocked(),
		Type:             input.Type,
		TargetSelector:   input.TargetSelector,
		Strategy:         normalizeStrategy(input.Strategy),
		Status:           status,
		Payload:          input.Payload,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        input.CreatedBy,
		RequiresApproval: approval,
		Summary:          input.Summary,
		MatchedNodes:     matched,
		Rollout: domain.RolloutProgress{
			Mode:           rolloutMode(input.Strategy),
			TotalNodes:     len(matched),
			CompletedNodes: 0,
			FailedNodes:    0,
			Batches:        []domain.RolloutBatch{},
		},
		Plan: plan,
	}

	if job.Summary == "" {
		job.Summary = fmt.Sprintf("Execute %s on %s", job.Type, job.TargetSelector)
	}

	s.jobs = append(s.jobs, job)
	s.appendAuditLocked(input.CreatedBy, "create", "job", job.ID, "Job created", map[string]any{
		"type":             job.Type,
		"target_selector":  job.TargetSelector,
		"requiresApproval": job.RequiresApproval,
	})
	s.saveLocked()
	return job
}

func (s *MemoryStore) DeleteJob(id, actor string) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx := range s.jobs {
		if s.jobs[idx].ID != id {
			continue
		}

		job := s.jobs[idx]
		s.jobs = append(s.jobs[:idx], s.jobs[idx+1:]...)
		s.appendAuditLocked(actor, "delete", "job", id, "Job deleted", map[string]any{
			"type":   job.Type,
			"status": job.Status,
		})
		s.saveLocked()
		return job, nil
	}

	return domain.Job{}, ErrJobNotFound
}

func (s *MemoryStore) ApproveJob(id, actor string) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx := range s.jobs {
		if s.jobs[idx].ID != id {
			continue
		}

		if s.jobs[idx].Status != "awaiting_approval" {
			return domain.Job{}, ErrJobCannotApprove
		}

		now := time.Now().UTC()
		s.jobs[idx].Status = "approved"
		s.jobs[idx].ApprovedBy = actor
		s.jobs[idx].ApprovedAt = &now
		s.appendAuditLocked(actor, "approve", "job", id, "Job approved", map[string]any{
			"type": s.jobs[idx].Type,
		})
		s.saveLocked()
		return s.jobs[idx], nil
	}

	return domain.Job{}, ErrJobNotFound
}

func (s *MemoryStore) StartJob(id, actor string) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx := range s.jobs {
		if s.jobs[idx].ID != id {
			continue
		}

		job := &s.jobs[idx]
		if job.RequiresApproval && job.Status == "awaiting_approval" {
			return domain.Job{}, ErrJobNeedsApproval
		}
		if job.Status != "approved" && job.Status != "planned" {
			return domain.Job{}, ErrJobCannotStart
		}

		now := time.Now().UTC()
		job.Status = "running"
		job.StartedAt = &now
		job.Rollout.Mode = rolloutMode(job.Strategy)
		job.Rollout.TotalNodes = len(job.MatchedNodes)
		simulateRollout(job, s.nodes)

		if job.Status == "running" {
			job.Status = "completed"
			done := time.Now().UTC()
			job.CompletedAt = &done
		}

		s.appendAuditLocked(actor, "start", "job", id, "Job execution started", map[string]any{
			"status": job.Status,
		})
		s.saveLocked()
		return *job, nil
	}

	return domain.Job{}, ErrJobNotFound
}

func (s *MemoryStore) ListAuditEvents() []domain.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.AuditEvent, len(s.audits))
	copy(out, s.audits)
	slices.Reverse(out)
	return out
}

func (s *MemoryStore) DashboardSummary() domain.DashboardSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var summary domain.DashboardSummary
	summary.TotalNodes = len(s.nodes)
	summary.AuditEvents = len(s.audits)

	var totalCPU, totalRAM, totalDisk int
	for _, node := range s.nodes {
		if node.Status == "online" {
			summary.OnlineNodes++
		}
		if node.Status == "degraded" {
			summary.DegradedNodes++
		}

		totalCPU += node.Metrics.CPUPercent
		totalRAM += node.Metrics.RAMPercent
		totalDisk += node.Metrics.DiskPercent
	}

	for _, job := range s.jobs {
		if job.Status == "running" {
			summary.RunningJobs++
		}
		if job.Status == "awaiting_approval" {
			summary.AwaitingApprovals++
		}
	}

	if len(s.nodes) > 0 {
		summary.AverageCPU = round1(float64(totalCPU) / float64(len(s.nodes)))
		summary.AverageRAM = round1(float64(totalRAM) / float64(len(s.nodes)))
		summary.AverageDisk = round1(float64(totalDisk) / float64(len(s.nodes)))
	}

	return summary
}

func (s *MemoryStore) GetAISettings() domain.AISettings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.aiSettings
}

func (s *MemoryStore) UpdateAISettings(input domain.AIConfigUpdateInput) domain.AISettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.aiSettings

	if strings.TrimSpace(input.Provider) != "" {
		next.Provider = input.Provider
	}
	if strings.TrimSpace(input.GeminiModel) != "" {
		next.GeminiModel = input.GeminiModel
	}
	if strings.TrimSpace(input.GeminiBaseURL) != "" {
		next.GeminiBaseURL = input.GeminiBaseURL
	}

	if input.ClearGeminiAPIKey {
		next.GeminiAPIKey = ""
	} else if strings.TrimSpace(input.GeminiAPIKey) != "" {
		next.GeminiAPIKey = input.GeminiAPIKey
	}

	next = normalizeAISettings(next)
	s.aiSettings = next
	s.appendAuditLocked("operator", "ai-config-update", "ai", "provider", "AI provider settings updated", map[string]any{
		"provider": next.Provider,
		"model":    next.GeminiModel,
	})
	s.saveLocked()
	return s.aiSettings
}

func (s *MemoryStore) appendAudit(actor, action, resourceType, resourceID, message string, metadata map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendAuditLocked(actor, action, resourceType, resourceID, message, metadata)
	s.saveLocked()
}

func (s *MemoryStore) appendAuditLocked(actor, action, resourceType, resourceID, message string, metadata map[string]any) {
	event := domain.AuditEvent{
		ID:           fmt.Sprintf("audit-%d", len(s.audits)+1),
		Actor:        actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Message:      message,
		Metadata:     metadata,
		CreatedAt:    time.Now().UTC(),
	}
	s.audits = append(s.audits, event)
}

func (s *MemoryStore) saveLocked() {
	if s.persistPath == "" {
		return
	}

	if err := saveState(s.persistPath, persistedState{
		Nodes:         s.nodes,
		MetricSamples: s.metricSamples,
		Jobs:          s.jobs,
		Audits:        s.audits,
		AISettings:    s.aiSettings,
	}); err != nil {
		return
	}
}

func loadState(path string) (persistedState, error) {
	var state persistedState

	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}

	return state, nil
}

func saveState(path string, state persistedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func sanitizeSeededDemoState(state persistedState) (persistedState, bool) {
	if !hasDemoBootstrapAudit(state.Audits) {
		return state, false
	}

	filteredNodes := make([]domain.Node, 0, len(state.Nodes))
	for _, node := range state.Nodes {
		if node.ID == "node-argentina-17" || node.ID == "node-moscow-01" || node.ID == "node-berlin-05" {
			continue
		}
		filteredNodes = append(filteredNodes, node)
	}

	filteredJobs := make([]domain.Job, 0, len(state.Jobs))
	for _, job := range state.Jobs {
		if job.ID == "job-boot-1" && job.CreatedBy == "system" {
			continue
		}
		filteredJobs = append(filteredJobs, job)
	}

	filteredAudits := make([]domain.AuditEvent, 0, len(state.Audits))
	for _, audit := range state.Audits {
		if isDemoBootstrapAudit(audit) {
			continue
		}
		filteredAudits = append(filteredAudits, audit)
	}

	changed := len(filteredNodes) != len(state.Nodes) ||
		len(filteredJobs) != len(state.Jobs) ||
		len(filteredAudits) != len(state.Audits)

	if !changed {
		return state, false
	}

	return persistedState{
		Nodes:         filteredNodes,
		MetricSamples: state.MetricSamples,
		Jobs:          filteredJobs,
		Audits:        filteredAudits,
		AISettings:    state.AISettings,
	}, true
}

func hasDemoBootstrapAudit(audits []domain.AuditEvent) bool {
	for _, audit := range audits {
		if isDemoBootstrapAudit(audit) {
			return true
		}
	}
	return false
}

func isDemoBootstrapAudit(audit domain.AuditEvent) bool {
	return audit.Action == "bootstrap" &&
		audit.Actor == "system" &&
		audit.ResourceType == "fleet" &&
		audit.ResourceID == "seed" &&
		audit.Message == "Initialized in-memory demo data"
}

func (s *MemoryStore) nextJobIDLocked() string {
	maxID := 0
	for _, job := range s.jobs {
		if !strings.HasPrefix(job.ID, "job-") {
			continue
		}

		rawNumber := strings.TrimPrefix(job.ID, "job-")
		id, err := strconv.Atoi(rawNumber)
		if err != nil {
			continue
		}

		if id > maxID {
			maxID = id
		}
	}

	return fmt.Sprintf("job-%d", maxID+1)
}

func (s *MemoryStore) nextNodeIDLocked() string {
	maxID := 0
	for _, node := range s.nodes {
		if !strings.HasPrefix(node.ID, "node-") {
			continue
		}

		rawNumber := strings.TrimPrefix(node.ID, "node-")
		id, err := strconv.Atoi(rawNumber)
		if err != nil {
			continue
		}

		if id > maxID {
			maxID = id
		}
	}

	return fmt.Sprintf("node-%d", maxID+1)
}

func (s *MemoryStore) deduplicateNodesByIPLocked(actor string) int {
	deduped, removed := deduplicateNodesByIP(s.nodes)
	if removed == 0 {
		return 0
	}

	s.nodes = deduped
	s.appendAuditLocked(actor, "dedupe", "node", "inventory", "Removed duplicated nodes by ip_address", map[string]any{
		"removed": removed,
	})
	return removed
}

func deduplicateNodesByIP(nodes []domain.Node) ([]domain.Node, int) {
	if len(nodes) < 2 {
		return nodes, 0
	}

	out := make([]domain.Node, 0, len(nodes))
	seenByIP := map[string]int{}
	removed := 0

	for _, node := range nodes {
		ip := strings.TrimSpace(node.IPAddress)
		node.IPAddress = ip
		if ip == "" {
			out = append(out, node)
			continue
		}

		if existingIdx, exists := seenByIP[ip]; exists {
			kept := pickPreferredNode(out[existingIdx], node)
			other := node
			if kept.ID == node.ID {
				other = out[existingIdx]
			}

			out[existingIdx] = mergeNodeRecords(kept, other)
			removed++
			continue
		}

		seenByIP[ip] = len(out)
		out = append(out, node)
	}

	return out, removed
}

func pickPreferredNode(left, right domain.Node) domain.Node {
	if right.LastSeenAt.After(left.LastSeenAt) {
		return right
	}
	if left.LastSeenAt.After(right.LastSeenAt) {
		return left
	}
	if right.CreatedAt.After(left.CreatedAt) {
		return right
	}
	if left.CreatedAt.After(right.CreatedAt) {
		return left
	}

	if strings.TrimSpace(right.DisplayName) != "" && strings.TrimSpace(left.DisplayName) == "" {
		return right
	}
	return left
}

func mergeNodeRecords(primary, secondary domain.Node) domain.Node {
	merged := primary

	if strings.TrimSpace(merged.DisplayName) == "" {
		merged.DisplayName = secondary.DisplayName
	}
	if strings.TrimSpace(merged.Name) == "" {
		merged.Name = secondary.Name
	}
	if strings.TrimSpace(merged.Region) == "" {
		merged.Region = secondary.Region
	}
	if strings.TrimSpace(merged.Status) == "" {
		merged.Status = secondary.Status
	}
	if len(merged.Tags) == 0 {
		merged.Tags = secondary.Tags
	}
	if merged.Metrics == (domain.NodeMetrics{}) {
		merged.Metrics = secondary.Metrics
	}
	if merged.LastSeenAt.IsZero() {
		merged.LastSeenAt = secondary.LastSeenAt
	}
	if merged.CreatedAt.IsZero() || (!secondary.CreatedAt.IsZero() && secondary.CreatedAt.Before(merged.CreatedAt)) {
		merged.CreatedAt = secondary.CreatedAt
	}
	if strings.TrimSpace(merged.IPAddress) == "" {
		merged.IPAddress = strings.TrimSpace(secondary.IPAddress)
	}

	return merged
}

func (s *MemoryStore) resolveTargetNamesLocked(selector string) []string {
	selector = strings.TrimSpace(selector)
	if selector == "" || selector == "fleet:all" {
		return collectNodeNames(s.nodes)
	}

	if strings.HasPrefix(selector, "node:") {
		name := strings.TrimPrefix(selector, "node:")
		for _, node := range s.nodes {
			if node.Name == name || node.DisplayName == name || node.ID == name {
				return []string{node.Name}
			}
		}
		return nil
	}

	if strings.HasPrefix(selector, "tag:") {
		tag := strings.TrimPrefix(selector, "tag:")
		var matched []string
		for _, node := range s.nodes {
			if slices.Contains(node.Tags, tag) {
				matched = append(matched, node.Name)
			}
		}
		return matched
	}

	return nil
}

func collectNodeNames(nodes []domain.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.Name)
	}
	return out
}

func normalizeStrategy(strategy string) string {
	if strategy == "" {
		return "all-at-once"
	}
	if strings.HasPrefix(strategy, "rolling:") {
		return strategy
	}
	if strategy == "rolling" {
		return "rolling:10,25,50,100"
	}
	return strategy
}

func rolloutMode(strategy string) string {
	if strings.HasPrefix(strategy, "rolling") {
		return "rolling"
	}
	return "all-at-once"
}

func parseStrategy(strategy string, total int) []int {
	if total <= 0 {
		return []int{100}
	}

	if !strings.HasPrefix(strategy, "rolling:") {
		return []int{100}
	}

	raw := strings.TrimPrefix(strategy, "rolling:")
	parts := strings.Split(raw, ",")
	var out []int
	for _, part := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v <= 0 || v > 100 {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 || out[len(out)-1] != 100 {
		out = append(out, 100)
	}
	return out
}

func simulateRollout(job *domain.Job, nodes []domain.Node) {
	targeted := resolveNodesForJob(job.MatchedNodes, nodes)
	checkpoints := parseStrategy(job.Strategy, len(targeted))
	seen := map[string]bool{}

	for idx, percent := range checkpoints {
		batchNodes := cumulativeBatch(targeted, percent, seen)
		if len(batchNodes) == 0 {
			continue
		}

		batch := domain.RolloutBatch{
			Label:         fmt.Sprintf("%d%%", percent),
			TargetedNodes: collectNodeNames(batchNodes),
			Status:        "completed",
			UpdatedAt:     time.Now().UTC(),
		}

		for _, node := range batchNodes {
			if node.Status == "degraded" || slices.Contains(node.Tags, "legacy") {
				batch.FailedNodes++
				job.Rollout.FailedNodes++
				continue
			}
			batch.CompletedNodes++
			job.Rollout.CompletedNodes++
		}

		if batch.FailedNodes > 0 && job.Rollout.Mode == "rolling" {
			batch.Status = "paused"
			job.Rollout.Batches = append(job.Rollout.Batches, batch)
			job.Rollout.CurrentBatchIndex = idx + 1
			job.Rollout.CurrentBatchLabel = batch.Label
			job.Status = "paused"
			return
		}

		job.Rollout.Batches = append(job.Rollout.Batches, batch)
		job.Rollout.CurrentBatchIndex = idx + 1
		job.Rollout.CurrentBatchLabel = batch.Label
	}
}

func resolveNodesForJob(matched []string, nodes []domain.Node) []domain.Node {
	var out []domain.Node
	for _, name := range matched {
		for _, node := range nodes {
			if node.Name == name {
				out = append(out, node)
				break
			}
		}
	}
	return out
}

func cumulativeBatch(nodes []domain.Node, percent int, seen map[string]bool) []domain.Node {
	cutoff := (len(nodes) * percent) / 100
	if cutoff == 0 && len(nodes) > 0 {
		cutoff = 1
	}
	if cutoff > len(nodes) {
		cutoff = len(nodes)
	}

	var batch []domain.Node
	for _, node := range nodes[:cutoff] {
		if seen[node.Name] {
			continue
		}
		seen[node.Name] = true
		batch = append(batch, node)
	}
	return batch
}

func (s *MemoryStore) appendMetricSampleLocked(node domain.Node, now time.Time) {
	if s.lastMetricSampleAt == nil {
		s.lastMetricSampleAt = map[string]time.Time{}
	}

	last := s.lastMetricSampleAt[node.Name]
	if !last.IsZero() && now.Sub(last) < metricSampleMinInterval {
		return
	}

	s.metricSamples = append(s.metricSamples, domain.NodeMetricSample{
		NodeName:   node.Name,
		Timestamp:  now,
		Status:     node.Status,
		Region:     node.Region,
		Tags:       node.Tags,
		Metrics:    node.Metrics,
		LastSeenAt: node.LastSeenAt,
	})
	s.lastMetricSampleAt[node.Name] = now

	pruned, removed := pruneMetricSamples(s.metricSamples, now, metricHistoryRetention)
	if removed > 0 {
		s.metricSamples = pruned
	}
}

func pruneMetricSamples(samples []domain.NodeMetricSample, now time.Time, retention time.Duration) ([]domain.NodeMetricSample, int) {
	if len(samples) == 0 || retention <= 0 {
		return samples, 0
	}

	cutoff := now.Add(-retention)
	out := make([]domain.NodeMetricSample, 0, len(samples))
	removed := 0
	for _, sample := range samples {
		if sample.Timestamp.Before(cutoff) {
			removed++
			continue
		}
		out = append(out, sample)
	}
	return out, removed
}

func buildLastMetricSampleIndex(samples []domain.NodeMetricSample) map[string]time.Time {
	index := map[string]time.Time{}
	for _, sample := range samples {
		if sample.Timestamp.After(index[sample.NodeName]) {
			index[sample.NodeName] = sample.Timestamp
		}
	}
	return index
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fallbackTags(value, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}
