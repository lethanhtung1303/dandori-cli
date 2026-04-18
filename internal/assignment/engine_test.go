package assignment

import (
	"testing"
)

func TestEngineSuggest(t *testing.T) {
	agents := []AgentConfig{
		{
			Name:                "alpha",
			Capabilities:        []string{"backend", "api"},
			PreferredIssueTypes: []string{"Bug"},
			MaxConcurrent:       3,
			ActiveRuns:          0,
			Active:              true,
		},
		{
			Name:                "beta",
			Capabilities:        []string{"frontend", "react"},
			PreferredIssueTypes: []string{"Story"},
			MaxConcurrent:       3,
			ActiveRuns:          0,
			Active:              true,
		},
	}

	task := Task{
		IssueKey:  "TEST-1",
		IssueType: "Bug",
		Labels:    []string{"backend", "api"},
	}

	engine := NewEngine(nil) // no history provider for this test
	suggestions := engine.Suggest(task, agents)

	if len(suggestions) == 0 {
		t.Fatal("expected suggestions")
	}

	// Alpha should be ranked higher (backend/api match + Bug preference)
	if suggestions[0].AgentName != "alpha" {
		t.Errorf("top suggestion = %s, want alpha", suggestions[0].AgentName)
	}

	t.Logf("Suggestions:")
	for _, s := range suggestions {
		t.Logf("  %s: score=%d reason=%s", s.AgentName, s.Score, s.Reason)
	}
}

func TestEngineSuggestLoadBalance(t *testing.T) {
	agents := []AgentConfig{
		{
			Name:          "alpha",
			Capabilities:  []string{"backend"},
			MaxConcurrent: 3,
			ActiveRuns:    2, // heavily loaded
			Active:        true,
		},
		{
			Name:          "beta",
			Capabilities:  []string{"backend"},
			MaxConcurrent: 3,
			ActiveRuns:    0, // idle
			Active:        true,
		},
	}

	task := Task{
		IssueKey:  "TEST-1",
		IssueType: "Task",
		Labels:    []string{"backend"},
	}

	engine := NewEngine(nil)
	suggestions := engine.Suggest(task, agents)

	// Beta should rank higher due to load balancing
	if suggestions[0].AgentName != "beta" {
		t.Errorf("top suggestion = %s, want beta (lower load)", suggestions[0].AgentName)
	}
}

func TestEngineSuggestNoActiveAgents(t *testing.T) {
	agents := []AgentConfig{
		{Name: "alpha", Active: false},
	}

	task := Task{IssueKey: "TEST-1"}

	engine := NewEngine(nil)
	suggestions := engine.Suggest(task, agents)

	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for inactive agents, got %d", len(suggestions))
	}
}

func TestEngineSuggestAtCapacity(t *testing.T) {
	agents := []AgentConfig{
		{
			Name:          "alpha",
			Capabilities:  []string{"backend"},
			MaxConcurrent: 2,
			ActiveRuns:    2, // at capacity
			Active:        true,
		},
	}

	task := Task{
		IssueKey: "TEST-1",
		Labels:   []string{"backend"},
	}

	engine := NewEngine(nil)
	suggestions := engine.Suggest(task, agents)

	// Should still return suggestion but load balance component is 0
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion even at capacity")
	}
	// At capacity: load balance (10%) = 0, so max possible = 90
	// Actual: capability 40% + neutral type 15% + neutral history 10% = 65
	if suggestions[0].Score > 70 {
		t.Errorf("score = %d, expected reduced score for agent at capacity", suggestions[0].Score)
	}
	t.Logf("At capacity score: %d", suggestions[0].Score)
}

type mockHistoryProvider struct {
	stats map[string]HistoryStats
}

func (m *mockHistoryProvider) GetStats(agentName, issueType string) HistoryStats {
	key := agentName + ":" + issueType
	if s, ok := m.stats[key]; ok {
		return s
	}
	return HistoryStats{}
}

func TestEngineSuggestWithHistory(t *testing.T) {
	agents := []AgentConfig{
		{
			Name:          "alpha",
			Capabilities:  []string{"backend"},
			MaxConcurrent: 3,
			Active:        true,
		},
		{
			Name:          "beta",
			Capabilities:  []string{"backend"},
			MaxConcurrent: 3,
			Active:        true,
		},
	}

	task := Task{
		IssueKey:  "TEST-1",
		IssueType: "Bug",
		Labels:    []string{"backend"},
	}

	history := &mockHistoryProvider{
		stats: map[string]HistoryStats{
			"alpha:Bug": {TotalRuns: 10, SuccessRuns: 9, SuccessRate: 0.9},
			"beta:Bug":  {TotalRuns: 10, SuccessRuns: 5, SuccessRate: 0.5},
		},
	}

	engine := NewEngine(history)
	suggestions := engine.Suggest(task, agents)

	// Alpha should rank higher due to better history
	if suggestions[0].AgentName != "alpha" {
		t.Errorf("top = %s, want alpha (better history)", suggestions[0].AgentName)
	}
}
