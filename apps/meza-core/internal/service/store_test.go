package service

import (
	"testing"

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
