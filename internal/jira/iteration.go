package jira

import "time"

// PriorRun is the slice of dandori run state that DetectIteration needs.
// Defined here (not pulled from db package) so the detection function stays
// pure and independent — the poller adapts its db.Run rows into this shape.
type PriorRun struct {
	RunID                    string
	Status                   string // dandori run status (done|error|cancelled|running)
	JiraStatusAtCompletion   string // Jira status name when run finished
	JiraCategoryAtCompletion string // Jira statusCategory.key (done|indeterminate|new)
	EndedAt                  time.Time
}

// IterationEvent is the payload DetectIteration returns when a Done→Active
// transition is found. The poller wraps this into a recorder.RecordEvent call.
type IterationEvent struct {
	Round          int
	IssueKey       string
	PrevRunID      string
	PrevStatus     string
	NewStatus      string
	TransitionedAt time.Time
}

// Payload returns the JSON-friendly map persisted into events.data.
func (e *IterationEvent) Payload() map[string]any {
	return map[string]any{
		"round":           e.Round,
		"issue_key":       e.IssueKey,
		"prev_run_id":     e.PrevRunID,
		"prev_status":     e.PrevStatus,
		"new_status":      e.NewStatus,
		"transitioned_at": e.TransitionedAt.Format(time.RFC3339),
	}
}

// doneCategories / activeCategories use Jira statusCategory.key — not status
// name — because workflows differ per company (Closed/Resolved/Verified all
// map to category=done; In Progress/To Do map to indeterminate/new).
var (
	doneCategories   = map[string]bool{"done": true}
	activeCategories = map[string]bool{"indeterminate": true, "new": true}
)

// DetectIteration returns an IterationEvent when issue's current status
// represents a regression from a previously-completed run, otherwise nil.
//
// Rules:
//   - lastRun nil → first time we've seen this issue, not an iteration.
//   - lastRun's Jira category at completion not in doneCategories → previous
//     run didn't actually finish the task, so the active status today is just
//     continuation, not iteration.
//   - issue category not in activeCategories → still done, no transition.
//   - existingEvents already contains an event with TransitionedAt ==
//     issue.UpdatedAt → poller already caught this transition, dedupe.
//
// Round = highest existing round + 1, defaulting to 2 (round 1 is the
// implicit first run).
func DetectIteration(issue *Issue, lastRun *PriorRun, existingEvents []IterationEvent) (*IterationEvent, error) {
	if issue == nil || lastRun == nil {
		return nil, nil
	}
	if !doneCategories[lastRun.JiraCategoryAtCompletion] {
		return nil, nil
	}
	if !activeCategories[issue.StatusCategoryKey] {
		return nil, nil
	}

	transitionedAt := issue.UpdatedAt
	for _, ev := range existingEvents {
		if ev.TransitionedAt.Equal(transitionedAt) {
			return nil, nil
		}
	}

	round := 2
	for _, ev := range existingEvents {
		if ev.Round >= round {
			round = ev.Round + 1
		}
	}

	return &IterationEvent{
		Round:          round,
		IssueKey:       issue.Key,
		PrevRunID:      lastRun.RunID,
		PrevStatus:     lastRun.JiraStatusAtCompletion,
		NewStatus:      issue.Status,
		TransitionedAt: transitionedAt,
	}, nil
}
