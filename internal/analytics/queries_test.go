package analytics

import (
	"context"
	"testing"
	"time"
)

type mockDB struct {
	runs []mockRun
}

type mockRun struct {
	ID           string
	AgentName    string
	SprintID     string
	IssueKey     string
	IssueType    string
	Status       string
	ExitCode     int
	CostUSD      float64
	DurationSec  float64
	InputTokens  int
	OutputTokens int
	StartedAt    time.Time
	StoryPoints  float64
}

func (m *mockDB) QueryAgentStats(ctx context.Context, f Filters, groupBy string) ([]AgentStat, error) {
	stats := make(map[string]*AgentStat)
	for _, r := range m.runs {
		if f.Agent != "" && r.AgentName != f.Agent {
			continue
		}
		if f.From != nil && r.StartedAt.Before(*f.From) {
			continue
		}
		if f.To != nil && r.StartedAt.After(*f.To) {
			continue
		}

		key := r.AgentName
		if _, ok := stats[key]; !ok {
			stats[key] = &AgentStat{AgentName: r.AgentName}
		}
		s := stats[key]
		s.RunCount++
		s.TotalCost += r.CostUSD
		s.AvgDuration = (s.AvgDuration*float64(s.RunCount-1) + r.DurationSec) / float64(s.RunCount)
		if r.ExitCode == 0 {
			s.SuccessRate = float64(int(s.SuccessRate*float64(s.RunCount-1)/100)+1) * 100 / float64(s.RunCount)
		}
	}

	result := make([]AgentStat, 0, len(stats))
	for _, s := range stats {
		s.AvgCost = s.TotalCost / float64(s.RunCount)
		result = append(result, *s)
	}
	return result, nil
}

func (m *mockDB) QueryCostBreakdown(ctx context.Context, f Filters, groupBy string) ([]CostGroup, error) {
	groups := make(map[string]*CostGroup)
	for _, r := range m.runs {
		if f.SprintID != "" && r.SprintID != f.SprintID {
			continue
		}
		if f.From != nil && r.StartedAt.Before(*f.From) {
			continue
		}

		var key string
		switch groupBy {
		case "agent":
			key = r.AgentName
		case "sprint":
			key = r.SprintID
		case "task":
			key = r.IssueKey
		default:
			key = "all"
		}

		if _, ok := groups[key]; !ok {
			groups[key] = &CostGroup{Group: key}
		}
		g := groups[key]
		g.Cost += r.CostUSD
		g.RunCount++
		g.Tokens += r.InputTokens + r.OutputTokens
	}

	result := make([]CostGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}
	return result, nil
}

func TestAgentStatsBasic(t *testing.T) {
	now := time.Now()
	db := &mockDB{
		runs: []mockRun{
			{ID: "1", AgentName: "alpha", ExitCode: 0, CostUSD: 1.0, DurationSec: 60, StartedAt: now},
			{ID: "2", AgentName: "alpha", ExitCode: 0, CostUSD: 2.0, DurationSec: 120, StartedAt: now},
			{ID: "3", AgentName: "beta", ExitCode: 1, CostUSD: 0.5, DurationSec: 30, StartedAt: now},
		},
	}

	stats, err := db.QueryAgentStats(context.Background(), Filters{}, "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 2 {
		t.Errorf("expected 2 agents, got %d", len(stats))
	}

	// Find alpha stats
	var alpha *AgentStat
	for i := range stats {
		if stats[i].AgentName == "alpha" {
			alpha = &stats[i]
			break
		}
	}
	if alpha == nil {
		t.Fatal("alpha not found")
	}
	if alpha.RunCount != 2 {
		t.Errorf("alpha run count = %d, want 2", alpha.RunCount)
	}
	if alpha.TotalCost != 3.0 {
		t.Errorf("alpha total cost = %f, want 3.0", alpha.TotalCost)
	}
}

func TestAgentStatsWithFilter(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	db := &mockDB{
		runs: []mockRun{
			{ID: "1", AgentName: "alpha", CostUSD: 1.0, StartedAt: now},
			{ID: "2", AgentName: "alpha", CostUSD: 2.0, StartedAt: yesterday},
		},
	}

	from := now.Add(-1 * time.Hour)
	stats, err := db.QueryAgentStats(context.Background(), Filters{From: &from}, "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 1 {
		t.Errorf("expected 1 stat after filter, got %d", len(stats))
	}
	if stats[0].TotalCost != 1.0 {
		t.Errorf("cost = %f, want 1.0 (filtered)", stats[0].TotalCost)
	}
}

func TestCostBreakdownByAgent(t *testing.T) {
	now := time.Now()
	db := &mockDB{
		runs: []mockRun{
			{ID: "1", AgentName: "alpha", CostUSD: 10.0, InputTokens: 1000, OutputTokens: 500, StartedAt: now},
			{ID: "2", AgentName: "alpha", CostUSD: 5.0, InputTokens: 500, OutputTokens: 250, StartedAt: now},
			{ID: "3", AgentName: "beta", CostUSD: 8.0, InputTokens: 800, OutputTokens: 400, StartedAt: now},
		},
	}

	groups, err := db.QueryCostBreakdown(context.Background(), Filters{}, "agent")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}

	var alpha *CostGroup
	for i := range groups {
		if groups[i].Group == "alpha" {
			alpha = &groups[i]
			break
		}
	}
	if alpha == nil {
		t.Fatal("alpha group not found")
	}
	if alpha.Cost != 15.0 {
		t.Errorf("alpha cost = %f, want 15.0", alpha.Cost)
	}
	if alpha.RunCount != 2 {
		t.Errorf("alpha runs = %d, want 2", alpha.RunCount)
	}
	if alpha.Tokens != 2250 {
		t.Errorf("alpha tokens = %d, want 2250", alpha.Tokens)
	}
}

func TestCostBreakdownBySprint(t *testing.T) {
	now := time.Now()
	db := &mockDB{
		runs: []mockRun{
			{ID: "1", AgentName: "alpha", SprintID: "sprint-1", CostUSD: 10.0, StartedAt: now},
			{ID: "2", AgentName: "alpha", SprintID: "sprint-1", CostUSD: 5.0, StartedAt: now},
			{ID: "3", AgentName: "beta", SprintID: "sprint-2", CostUSD: 8.0, StartedAt: now},
		},
	}

	groups, err := db.QueryCostBreakdown(context.Background(), Filters{SprintID: "sprint-1"}, "agent")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	// Should only have sprint-1 runs
	totalCost := 0.0
	for _, g := range groups {
		totalCost += g.Cost
	}
	if totalCost != 15.0 {
		t.Errorf("total cost = %f, want 15.0 (sprint-1 only)", totalCost)
	}
}

func TestFiltersEmpty(t *testing.T) {
	f := Filters{}
	if f.From != nil || f.To != nil {
		t.Error("empty filters should have nil dates")
	}
	if f.Agent != "" || f.SprintID != "" {
		t.Error("empty filters should have empty strings")
	}
}

func TestAgentStatZeroRuns(t *testing.T) {
	db := &mockDB{runs: []mockRun{}}

	stats, err := db.QueryAgentStats(context.Background(), Filters{}, "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 stats for empty DB, got %d", len(stats))
	}
}

func TestCostBreakdownZeroRuns(t *testing.T) {
	db := &mockDB{runs: []mockRun{}}

	groups, err := db.QueryCostBreakdown(context.Background(), Filters{}, "agent")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for empty DB, got %d", len(groups))
	}
}
