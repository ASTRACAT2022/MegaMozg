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
