package assignment

import (
	"sort"
)

type Suggestion struct {
	AgentName string
	Score     int
	Reason    string
}

type HistoryProvider interface {
	GetStats(agentName, issueType string) HistoryStats
}

type Engine struct {
	history HistoryProvider
}

func NewEngine(history HistoryProvider) *Engine {
	return &Engine{history: history}
}

func (e *Engine) Suggest(task Task, agents []AgentConfig) []Suggestion {
	var suggestions []Suggestion

	for _, agent := range agents {
		if !agent.Active {
			continue
		}

		var history HistoryStats
		if e.history != nil {
			history = e.history.GetStats(agent.Name, task.IssueType)
		}

		score, reason := Score(agent, task, history)

		suggestions = append(suggestions, Suggestion{
			AgentName: agent.Name,
			Score:     score,
			Reason:    reason,
		})
	}

	// Sort by score descending, then by name (for stable ordering)
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		return suggestions[i].AgentName < suggestions[j].AgentName
	})

	return suggestions
}

func (e *Engine) SuggestTop(task Task, agents []AgentConfig) *Suggestion {
	suggestions := e.Suggest(task, agents)
	if len(suggestions) == 0 {
		return nil
	}
	return &suggestions[0]
}
