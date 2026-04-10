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
