package model

import (
	"database/sql"
	"testing"
	"time"
)

func TestRunStatus(t *testing.T) {
	tests := []struct {
		status     RunStatus
		isComplete bool
	}{
		{StatusRunning, false},
		{StatusDone, true},
		{StatusError, true},
		{StatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			run := &Run{Status: tt.status}
			if run.IsComplete() != tt.isComplete {
				t.Errorf("IsComplete() = %v, want %v", run.IsComplete(), tt.isComplete)
			}
		})
	}
}

func TestRunTotalTokens(t *testing.T) {
	run := &Run{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	if run.TotalTokens() != 1500 {
		t.Errorf("TotalTokens() = %d, want 1500", run.TotalTokens())
	}
}

func TestRunWithNullFields(t *testing.T) {
	run := &Run{
		ID:            "test-123",
		AgentName:     "alpha",
		AgentType:     "claude_code",
		User:          "phuc",
		WorkstationID: "ws-1",
		StartedAt:     time.Now(),
		Status:        StatusRunning,
		JiraIssueKey:  sql.NullString{Valid: false},
		JiraSprintID:  sql.NullString{Valid: false},
		CWD:           sql.NullString{String: "/home/user/project", Valid: true},
	}

	if run.JiraIssueKey.Valid {
		t.Error("JiraIssueKey should be null")
	}

	if !run.CWD.Valid {
		t.Error("CWD should be valid")
	}
}

func TestEventLayers(t *testing.T) {
	if LayerWrapper != 1 {
		t.Errorf("LayerWrapper = %d, want 1", LayerWrapper)
	}
	if LayerTailer != 2 {
		t.Errorf("LayerTailer = %d, want 2", LayerTailer)
	}
	if LayerSkill != 3 {
		t.Errorf("LayerSkill = %d, want 3", LayerSkill)
	}
}

func TestAuditActions(t *testing.T) {
	actions := []AuditAction{
		AuditRunStarted,
		AuditRunCompleted,
		AuditRunFailed,
		AuditTaskAssigned,
		AuditConfigChanged,
		AuditSyncCompleted,
	}

	seen := make(map[AuditAction]bool)
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate action: %s", a)
		}
		seen[a] = true

		if a == "" {
			t.Error("action should not be empty")
		}
	}
}

func TestWrapperEvent(t *testing.T) {
	event := WrapperEvent{
		Type:      "run_started",
		RunID:     "run-123",
		Command:   "claude fix bug",
		StartedAt: "2026-04-18T10:00:00Z",
	}

	if event.Type != "run_started" {
		t.Errorf("Type = %s, want run_started", event.Type)
	}
}

func TestTailerEvent(t *testing.T) {
	event := TailerEvent{
		Type:         "tokens_updated",
		RunID:        "run-123",
		SessionID:    "session-456",
		InputTokens:  5000,
		OutputTokens: 2000,
		CacheRead:    1000,
		CacheWrite:   500,
		Model:        "claude-sonnet-4-6",
		CostUSD:      0.05,
	}

	if event.InputTokens+event.OutputTokens != 7000 {
		t.Error("token sum incorrect")
	}
}

func TestSkillEvent(t *testing.T) {
	event := SkillEvent{
		Type:   "decision",
		RunID:  "run-123",
		Action: "chose_architecture",
		Details: map[string]string{
			"rationale": "REST is simpler for this use case",
		},
	}

	if event.Action != "chose_architecture" {
		t.Errorf("Action = %s, want chose_architecture", event.Action)
	}
}
