package service

import (
	"testing"
	"time"

	"github.com/mezamozg/meza-core/internal/domain"
)

func TestStorePersistsStateToDisk(t *testing.T) {
	path := t.TempDir() + "/state.json"

	store, err := NewMemoryStoreWithFile(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	created := store.CreateJob(domain.JobCreateInput{
		Type:           "package_refresh",
		TargetSelector: "tag:staging",
		Strategy:       "rolling:10,25,50,100",
		CreatedBy:      "tester",
		Summary:        "persist me",
	}, nil)

	reloaded, err := NewMemoryStoreWithFile(path)
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	job, err := reloaded.GetJob(created.ID)
	if err != nil {
		t.Fatalf("expected persisted job, got error: %v", err)
	}

	if job.Summary != "persist me" {
		t.Fatalf("expected persisted summary, got %q", job.Summary)
	}
}

func TestCreateJobIDDoesNotReuseAfterDelete(t *testing.T) {
	store := NewMemoryStore()

	first := store.CreateJob(domain.JobCreateInput{
		Type:           "manual_review",
		TargetSelector: "fleet:all",
		Strategy:       "all-at-once",
		CreatedBy:      "tester",
	}, nil)
	second := store.CreateJob(domain.JobCreateInput{
		Type:           "manual_review",
		TargetSelector: "fleet:all",
		Strategy:       "all-at-once",
		CreatedBy:      "tester",
	}, nil)

	if _, err := store.DeleteJob(first.ID, "tester"); err != nil {
		t.Fatalf("failed to delete job: %v", err)
	}

	third := store.CreateJob(domain.JobCreateInput{
		Type:           "manual_review",
		TargetSelector: "fleet:all",
		Strategy:       "all-at-once",
		CreatedBy:      "tester",
	}, nil)

	if second.ID != "job-2" {
		t.Fatalf("expected second id job-2, got %s", second.ID)
	}

	if third.ID != "job-3" {
		t.Fatalf("expected third id job-3 after deletion, got %s", third.ID)
	}
}

func TestNodeMetadataAndDisplayNameSelector(t *testing.T) {
	store := NewMemoryStore()

	registered := store.RegisterNode(domain.NodeRegisterInput{
		Name:      "argentina-17",
		Region:    "south-america",
		IPAddress: "203.0.113.10",
		Tags:      []string{"prod"},
	})

	updated, err := store.UpdateNode(registered.ID, domain.NodeUpdateInput{
		DisplayName: "Argentina Prod 17",
	}, "tester")
	if err != nil {
		t.Fatalf("expected update node to succeed, got error: %v", err)
	}

	if updated.DisplayName != "Argentina Prod 17" {
		t.Fatalf("expected display name to be updated, got %q", updated.DisplayName)
	}

	heartbeat := store.HeartbeatNode(domain.NodeHeartbeatInput{
		Name:      "argentina-17",
		Region:    "south-america",
		IPAddress: "198.51.100.20",
		Status:    "online",
		Metrics: domain.NodeMetrics{
			CPUPercent: 10,
			RAMPercent: 20,
		},
	})

	if heartbeat.IPAddress != "198.51.100.20" {
		t.Fatalf("expected ip to be updated from heartbeat, got %q", heartbeat.IPAddress)
	}

	job := store.CreateJob(domain.JobCreateInput{
		Type:           "manual_review",
		TargetSelector: "node:Argentina Prod 17",
		Strategy:       "all-at-once",
		CreatedBy:      "tester",
	}, nil)

	if len(job.MatchedNodes) != 1 || job.MatchedNodes[0] != "argentina-17" {
		t.Fatalf("expected selector by display name to match technical node name, got %+v", job.MatchedNodes)
	}
}

func TestRegisterNodeDeduplicatesByIPAddress(t *testing.T) {
	store := NewMemoryStore()

	first := store.RegisterNode(domain.NodeRegisterInput{
		Name:      "argentina-17",
		Region:    "south-america",
		IPAddress: "203.0.113.10",
		Tags:      []string{"prod"},
	})
	second := store.RegisterNode(domain.NodeRegisterInput{
		Name:      "argentina-17-reinstall",
		Region:    "south-america",
		IPAddress: "203.0.113.10",
		Tags:      []string{"prod", "reinstall"},
	})

	nodes := store.ListNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after dedupe by ip, got %d", len(nodes))
	}
	if nodes[0].ID != first.ID {
		t.Fatalf("expected to keep original id %q, got %q", first.ID, nodes[0].ID)
	}
	if second.ID != first.ID {
		t.Fatalf("expected register to update existing node id %q, got %q", first.ID, second.ID)
	}
	if nodes[0].Name != "argentina-17-reinstall" {
		t.Fatalf("expected latest node name to be stored, got %q", nodes[0].Name)
	}
}

func TestHeartbeatDeduplicatesByIPAddressWhenNameChanged(t *testing.T) {
	store := NewMemoryStore()

	store.RegisterNode(domain.NodeRegisterInput{
		Name:      "node-old-name",
		Region:    "ru",
		IPAddress: "198.51.100.20",
	})

	heartbeat := store.HeartbeatNode(domain.NodeHeartbeatInput{
		Name:      "node-new-name",
		Region:    "ru",
		IPAddress: "198.51.100.20",
		Status:    "online",
		Metrics: domain.NodeMetrics{
			CPUPercent: 45,
		},
	})

	nodes := store.ListNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after heartbeat dedupe, got %d", len(nodes))
	}
	if heartbeat.Name != "node-new-name" {
		t.Fatalf("expected heartbeat to update node name, got %q", heartbeat.Name)
	}
	if nodes[0].Name != "node-new-name" {
		t.Fatalf("expected stored node name node-new-name, got %q", nodes[0].Name)
	}
}

func TestStoreDeduplicatesPersistedNodesByIPAddressOnLoad(t *testing.T) {
	path := t.TempDir() + "/state.json"
	now := time.Now().UTC()

	state := persistedState{
		Nodes: []domain.Node{
			{
				ID:          "node-1",
				Name:        "first",
				DisplayName: "Primary Name",
				IPAddress:   "192.0.2.50",
				Region:      "ru",
				Status:      "online",
				LastSeenAt:  now.Add(-2 * time.Minute),
				CreatedAt:   now.Add(-10 * time.Minute),
			},
			{
				ID:         "node-2",
				Name:       "second",
				IPAddress:  "192.0.2.50",
				Region:     "ru",
				Status:     "online",
				LastSeenAt: now,
				CreatedAt:  now.Add(-5 * time.Minute),
			},
		},
		Jobs:       []domain.Job{},
		Audits:     []domain.AuditEvent{},
		AISettings: domain.AISettings{},
	}
	if err := saveState(path, state); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	reloaded, err := NewMemoryStoreWithFile(path)
	if err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	nodes := reloaded.ListNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after load dedupe, got %d", len(nodes))
	}
	if nodes[0].Name != "second" {
		t.Fatalf("expected newest node by heartbeat to stay, got %q", nodes[0].Name)
	}
	if nodes[0].DisplayName != "Primary Name" {
		t.Fatalf("expected display name to be preserved from duplicate, got %q", nodes[0].DisplayName)
	}
}
