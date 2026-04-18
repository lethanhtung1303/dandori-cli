package assignment

import (
	"fmt"
	"strings"
)

const (
	WeightCapability  = 0.40
	WeightIssueType   = 0.30
	WeightHistory     = 0.20
	WeightLoadBalance = 0.10
)

type AgentConfig struct {
	Name                string
	AgentType           string
	WorkstationID       string
	Capabilities        []string
	PreferredIssueTypes []string
	MaxConcurrent       int
	Team                string
	Active              bool
	ActiveRuns          int // populated at runtime
}

type Task struct {
	IssueKey    string
	Summary     string
	IssueType   string
	Priority    string
	Labels      []string
	Components  []string
	StoryPoints float64
}

type HistoryStats struct {
	TotalRuns   int
	SuccessRuns int
	SuccessRate float64 // 0.0 - 1.0
}

func Score(agent AgentConfig, task Task, history HistoryStats) (int, string) {
	capScore := scoreCapabilityOverlap(agent, task)
	typeScore := scoreIssueTypePreference(agent, task)
	histScore := scoreHistory(history)
	loadScore := scoreLoadBalance(agent)

	total := capScore*WeightCapability +
		typeScore*WeightIssueType +
		histScore*WeightHistory +
		loadScore*WeightLoadBalance

	finalScore := int(total * 100)

	reason := buildExplanation(agent, task, history, capScore, typeScore, histScore, loadScore)

	return finalScore, reason
}

func scoreCapabilityOverlap(agent AgentConfig, task Task) float64 {
	taskTerms := make(map[string]bool)
	for _, l := range task.Labels {
		taskTerms[strings.ToLower(l)] = true
	}
	for _, c := range task.Components {
		// Extract key part: "auth-service" -> "auth"
		parts := strings.Split(strings.ToLower(c), "-")
		for _, p := range parts {
			taskTerms[p] = true
		}
	}

	if len(taskTerms) == 0 {
		return 0.0
	}

	matches := 0
	for _, cap := range agent.Capabilities {
		capLower := strings.ToLower(cap)
		if taskTerms[capLower] {
			matches++
		}
	}

	return float64(matches) / float64(len(taskTerms))
}

func scoreIssueTypePreference(agent AgentConfig, task Task) float64 {
	if len(agent.PreferredIssueTypes) == 0 {
		return 0.5 // neutral
	}

	taskType := strings.ToLower(task.IssueType)
	for _, pref := range agent.PreferredIssueTypes {
		if strings.ToLower(pref) == taskType {
			return 1.0
		}
	}
	return 0.0
}

func scoreHistory(history HistoryStats) float64 {
	if history.TotalRuns == 0 {
		return 0.5 // neutral for no history
	}
	return history.SuccessRate
}

func scoreLoadBalance(agent AgentConfig) float64 {
	maxConc := agent.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 3 // default
	}

	if agent.ActiveRuns >= maxConc {
		return 0.0
	}

	return 1.0 - (float64(agent.ActiveRuns) / float64(maxConc))
}

func buildExplanation(agent AgentConfig, task Task, history HistoryStats,
	capScore, typeScore, histScore, loadScore float64) string {

	var parts []string

	// Capability match
	if capScore > 0 {
		matched := findMatchedCapabilities(agent, task)
		if len(matched) > 0 {
			parts = append(parts, fmt.Sprintf("capabilities match [%s]", strings.Join(matched, ", ")))
		}
	}

	// Issue type preference
	if typeScore == 1.0 {
		parts = append(parts, fmt.Sprintf("prefers %s type", task.IssueType))
	}

	// History
	if history.TotalRuns > 0 {
		parts = append(parts, fmt.Sprintf("%.0f%% success on %s (%d runs)",
			history.SuccessRate*100, task.IssueType, history.TotalRuns))
	} else {
		parts = append(parts, "no history yet")
	}

	// Load
	if loadScore == 1.0 {
		parts = append(parts, "no active runs")
	} else if loadScore > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d slots used", agent.ActiveRuns, agent.MaxConcurrent))
	} else {
		parts = append(parts, "at capacity")
	}

	return strings.Join(parts, ", ")
}

func findMatchedCapabilities(agent AgentConfig, task Task) []string {
	taskTerms := make(map[string]bool)
	for _, l := range task.Labels {
		taskTerms[strings.ToLower(l)] = true
	}
	for _, c := range task.Components {
		parts := strings.Split(strings.ToLower(c), "-")
		for _, p := range parts {
			taskTerms[p] = true
		}
	}

	var matched []string
	for _, cap := range agent.Capabilities {
		if taskTerms[strings.ToLower(cap)] {
			matched = append(matched, cap)
		}
	}
	return matched
}
