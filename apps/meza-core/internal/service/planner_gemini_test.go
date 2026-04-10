package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGeminiPlannerUsesFunctionCallResponse(t *testing.T) {
	planner := NewGeminiPlanner(GeminiPlannerConfig{
		APIKey:  "gemini-test-key",
		Model:   "gemini-2.5-flash",
		BaseURL: "https://example.invalid/v1beta",
		Timeout: 2 * time.Second,
	}, NewStubPlanner())

	planner.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("x-goog-api-key") != "gemini-test-key" {
				t.Fatalf("expected API key header")
			}

			return jsonResponse(`{
			  "candidates": [
			    {
			      "content": {
			        "parts": [
			          {
			            "functionCall": {
			              "name": "plan_typed_job",
			              "args": {
			                "intent": "update_docker",
			                "requires_review": true,
			                "summary": "Gemini planned a Docker update job.",
			                "job_preview": {
			                  "type": "update_docker",
			                  "target_selector": "node:argentina-17",
			                  "strategy": "rolling:10,25,50,100",
			                  "payload": {
			                    "channel": "stable"
			                  }
			                }
			              }
			            }
			          }
			        ]
			      }
			    }
			  ]
			}`), nil
		}),
	}

	plan := planner.Interpret("обнови docker на ноде argentina-17")

	if plan.Provider != "gemini" {
		t.Fatalf("expected provider gemini, got %q", plan.Provider)
	}
	if plan.Intent != "update_docker" {
		t.Fatalf("expected update_docker, got %q", plan.Intent)
	}
	if plan.JobPreview.TargetSelector != "node:argentina-17" {
		t.Fatalf("unexpected target selector %q", plan.JobPreview.TargetSelector)
	}
}

func TestGeminiPlannerFallsBackOnFailure(t *testing.T) {
	planner := NewGeminiPlanner(GeminiPlannerConfig{
		APIKey:  "gemini-test-key",
		Model:   "gemini-2.5-flash",
		BaseURL: "https://example.invalid/v1beta",
		Timeout: 2 * time.Second,
	}, NewStubPlanner())

	planner.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("boom")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	plan := planner.Interpret("обнови docker на ноде argentina-17")

	if plan.Provider != "gemini-error" {
		t.Fatalf("expected gemini-error provider, got %q", plan.Provider)
	}
	if plan.Intent != "planner_error" {
		t.Fatalf("expected planner_error intent, got %q", plan.Intent)
	}
}

func TestGeminiPlannerChatDoesNotSendEmptyToolConfig(t *testing.T) {
	planner := NewGeminiPlanner(GeminiPlannerConfig{
		APIKey:  "gemini-test-key",
		Model:   "gemini-2.5-flash",
		BaseURL: "https://example.invalid/v1beta",
		Timeout: 2 * time.Second,
	}, NewStubPlanner())

	planner.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}

			if strings.Contains(string(raw), `"toolConfig"`) {
				t.Fatalf("chat request must not include toolConfig, got: %s", string(raw))
			}

			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("invalid request json: %v", err)
			}

			return jsonResponse(`{
			  "candidates": [
			    {
			      "content": {
			        "parts": [
			          { "text": "Я AI-агент MezaMozg." }
			        ]
			      }
			    }
			  ]
			}`), nil
		}),
	}

	reply, err := planner.Chat("кто ты")
	if err != nil {
		t.Fatalf("expected chat reply without error, got: %v", err)
	}
	if !strings.Contains(reply, "MezaMozg") {
		t.Fatalf("unexpected chat reply: %q", reply)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}
}
