package jira

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Scenario 32: Empty base URL
func TestClientEmptyBaseURL(t *testing.T) {
	client := NewClient(ClientConfig{BaseURL: ""})
	_, err := client.GetIssue("PROJ-123")
	if err == nil {
		t.Error("empty base URL should fail")
	}
}

// Scenario 35: API rate limit (429)
func TestClientRateLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"PROJ-123","fields":{"summary":"test"}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	_, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("should retry after rate limit: %v", err)
	}
	if attempts < 3 {
		t.Errorf("should have retried, attempts = %d", attempts)
	}
}

// Scenario 36: API timeout
func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
		Timeout: 100 * time.Millisecond,
	})

	_, err := client.GetIssue("PROJ-123")
	if err == nil {
		t.Error("should timeout")
	}
}

// Scenario 38: Jira server down (503)
func TestClientServerDown(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	_, err := client.GetIssue("PROJ-123")
	if err == nil {
		t.Error("503 should eventually fail")
	}
	if attempts < 2 {
		t.Error("should have retried")
	}
}

// Scenario 39: Invalid credentials (401)
func TestClientInvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["Invalid credentials"]}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "wrong",
		Token:   "wrong",
		IsCloud: true,
	})

	_, err := client.GetIssue("PROJ-123")
	if err == nil {
		t.Error("401 should return error")
	}
}

// Scenario 40: Issue not found (404)
func TestClientIssueNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	_, err := client.GetIssue("NONEXISTENT-999")
	if err == nil {
		t.Error("404 should return error")
	}
}

// Scenario 43: Sprint with 0 issues
func TestClientEmptySprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"issues":[]}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	issues, err := client.GetSprintIssues(1)
	if err != nil {
		t.Fatalf("empty sprint failed: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

// Scenario 44: Issue with null fields
func TestClientNullFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"key": "PROJ-123",
			"fields": {
				"summary": "test",
				"description": null,
				"assignee": null,
				"labels": [],
				"priority": {"name": "High"},
				"status": {"name": "Open"},
				"issuetype": {"name": "Task"}
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	issue, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("null fields failed: %v", err)
	}
	if issue.Key != "PROJ-123" {
		t.Error("key should be parsed")
	}
}

// Additional: Data Center auth (Bearer token)
func TestClientDataCenterAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"PROJ-123","fields":{"summary":"test","status":{"name":"Open"},"issuetype":{"name":"Task"},"priority":{"name":"High"}}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "pat-token-123",
		IsCloud: false, // Data Center
	})

	_, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("DC auth failed: %v", err)
	}
	if authHeader != "Bearer pat-token-123" {
		t.Errorf("auth = %s, want Bearer", authHeader)
	}
}

// Additional: Poller callback execution
func TestPollerCallbacks(t *testing.T) {
	newTaskCalled := false
	assignedCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/agile/1.0/board/1/sprint" {
			w.Write([]byte(`{"values":[{"id":10,"name":"Sprint 1","state":"active"}]}`))
			return
		}
		if r.URL.Path == "/rest/agile/1.0/sprint/10/issue" {
			w.Write([]byte(`{"issues":[
				{"key":"NEW-1","fields":{"summary":"new","status":{"name":"Open"},"issuetype":{"name":"Task"},"priority":{"name":"High"}}},
				{"key":"ASSIGNED-1","fields":{"summary":"assigned","customfield_10100":"alpha","status":{"name":"Open"},"issuetype":{"name":"Task"},"priority":{"name":"High"}}}
			]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	poller := NewPoller(PollerConfig{
		Client:   client,
		BoardID:  1,
		Interval: time.Hour,
		OnNewTask: func(i Issue) {
			newTaskCalled = true
		},
		OnAssigned: func(i Issue) {
			assignedCalled = true
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	poller.Poll(ctx)

	// At least one callback should be called
	if !newTaskCalled && !assignedCalled {
		t.Error("at least one callback should be called")
	}
}

// Additional: Transition not found
func TestTransitionNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"transitions":[{"id":"1","name":"Start","to":{"name":"In Progress"}}]}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	err := client.TransitionTo("PROJ-123", "Nonexistent Status")
	if err == nil {
		t.Error("should fail for unknown transition")
	}
}
