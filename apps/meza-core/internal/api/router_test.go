package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mezamozg/meza-core/internal/domain"
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

func TestAIConfigEndpoints(t *testing.T) {
	planner := service.NewRuntimePlanner(service.RuntimePlannerConfig{
		Provider: domain.DefaultAIProvider,
		Fallback: service.NewStubPlanner(),
	})
	server := NewServer(service.NewMemoryStore(), planner, AuthConfig{})

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/ai/config", nil)
	getRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected GET config 200, got %d", getRes.Code)
	}

	updateBody := bytes.NewBufferString(`{
	  "provider":"gemini",
	  "gemini_api_key":"test-key",
	  "gemini_model":"gemini-2.5-flash",
	  "gemini_base_url":"https://generativelanguage.googleapis.com/v1beta"
	}`)
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/ai/config", updateBody)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected update config 200, got %d, body=%s", updateRes.Code, updateRes.Body.String())
	}

	if !bytes.Contains(updateRes.Body.Bytes(), []byte(`"provider": "gemini"`)) {
		t.Fatalf("expected provider gemini, got %s", updateRes.Body.String())
	}
	if !bytes.Contains(updateRes.Body.Bytes(), []byte(`"has_gemini_api_key": true`)) {
		t.Fatalf("expected has_gemini_api_key=true, got %s", updateRes.Body.String())
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

func TestNodeRegisterCapturesIPAndAllowsDisplayNameUpdate(t *testing.T) {
	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{})

	registerBody := bytes.NewBufferString(`{
	  "name":"node-a",
	  "region":"ru-central",
	  "tags":["prod"]
	}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", registerBody)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.RemoteAddr = "203.0.113.7:43123"
	registerRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRes, registerReq)

	if registerRes.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d", registerRes.Code)
	}
	if !bytes.Contains(registerRes.Body.Bytes(), []byte(`"ip_address": "203.0.113.7"`)) {
		t.Fatalf("expected registered node to include captured ip, got %s", registerRes.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(registerRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/nodes/"+created.ID, bytes.NewBufferString(`{"display_name":"DB Moscow 01"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRes, updateReq)

	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected update node 200, got %d", updateRes.Code)
	}
	if !bytes.Contains(updateRes.Body.Bytes(), []byte(`"display_name": "DB Moscow 01"`)) {
		t.Fatalf("expected display_name in response, got %s", updateRes.Body.String())
	}
}

func TestTerminalAgentPollAndResultEndpoints(t *testing.T) {
	agentExecHub.reset()
	t.Cleanup(agentExecHub.reset)

	server := NewServer(service.NewMemoryStore(), service.NewStubPlanner(), AuthConfig{
		NodeToken: "node-token",
	})

	task := agentExecHub.enqueue("astra-1", "echo hello from node")

	pollReq := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/agent/poll", bytes.NewBufferString(`{"node_name":"astra-1"}`))
	pollReq.Header.Set("Content-Type", "application/json")
	pollReq.Header.Set("Authorization", "Bearer node-token")
	pollRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(pollRes, pollReq)

	if pollRes.Code != http.StatusOK {
		t.Fatalf("expected poll 200, got %d", pollRes.Code)
	}
	if got := pollRes.Header().Get("X-Meza-Task-ID"); got != task.ID {
		t.Fatalf("expected task id %q, got %q", task.ID, got)
	}
	if strings.TrimSpace(pollRes.Body.String()) != "echo hello from node" {
		t.Fatalf("expected command body, got %q", pollRes.Body.String())
	}

	resultReq := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/agent/result/"+task.ID+"?status=completed&exit_code=0", bytes.NewBufferString("ok"))
	resultReq.Header.Set("Authorization", "Bearer node-token")
	resultRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(resultRes, resultReq)

	if resultRes.Code != http.StatusOK {
		t.Fatalf("expected result 200, got %d", resultRes.Code)
	}

	snapshot, ok := agentExecHub.get(task.ID)
	if !ok {
		t.Fatalf("expected task snapshot to exist")
	}
	if snapshot.Status != "completed" {
		t.Fatalf("expected completed status, got %q", snapshot.Status)
	}
	if snapshot.Output != "ok" {
		t.Fatalf("expected output 'ok', got %q", snapshot.Output)
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
