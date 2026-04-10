package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mezamozg/meza-core/internal/service"
)

func TestHealthEndpoint(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}

func TestAIInterpretEndpoint(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{})

	body := bytes.NewBufferString(`{"prompt":"обнови docker на ноде argentina-17"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/interpret", body)
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	if !bytes.Contains(res.Body.Bytes(), []byte(`"intent": "update_docker"`)) {
		t.Fatalf("expected response to include update_docker plan, got %s", res.Body.String())
	}
}

func TestJobLifecycleEndpoints(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{})

	createBody := bytes.NewBufferString(`{
	  "type":"update_docker",
	  "target_selector":"node:argentina-17",
	  "strategy":"rolling:10,25,50,100",
	  "created_by":"tester"
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d", createRes.Code)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode created job: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/start", bytes.NewBufferString(`{"actor":"tester"}`))
	startRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRes, startReq)

	if startRes.Code != http.StatusConflict {
		t.Fatalf("expected start to fail with 409 before approval, got %d", startRes.Code)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/approve", bytes.NewBufferString(`{"actor":"reviewer"}`))
	approveRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(approveRes, approveReq)

	if approveRes.Code != http.StatusOK {
		t.Fatalf("expected approval 200, got %d", approveRes.Code)
	}

	startReq = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/start", bytes.NewBufferString(`{"actor":"tester"}`))
	startRes = httptest.NewRecorder()
	server.Handler().ServeHTTP(startRes, startReq)

	if startRes.Code != http.StatusOK {
		t.Fatalf("expected start 200 after approval, got %d", startRes.Code)
	}

	if !bytes.Contains(startRes.Body.Bytes(), []byte(`"status": "completed"`)) {
		t.Fatalf("expected job to complete, got %s", startRes.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+created.ID, bytes.NewBufferString(`{"actor":"tester"}`))
	deleteRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRes, deleteReq)

	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", deleteRes.Code)
	}
}

func TestDashboardEndpoint(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", res.Code)
	}

	if !bytes.Contains(res.Body.Bytes(), []byte(`"total_nodes"`)) {
		t.Fatalf("expected dashboard payload, got %s", res.Body.String())
	}
}

func TestOperatorAuth(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{
		OperatorToken: "secret-operator-token",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer secret-operator-token")
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", res.Code)
	}
}
