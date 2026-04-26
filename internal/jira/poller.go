package jira

import (
	"context"
	"log/slog"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/event"
	"github.com/phuc-nt/dandori-cli/internal/model"
)

type Poller struct {
	client          *Client
	boardID         int
	interval        time.Duration
	lastIssueSet    map[string]bool
	pendingSuggests map[string]pendingSuggest
	onNewTask       func(Issue)
	onAssigned      func(Issue)
	onSuggestAgent  func(Issue) (agentName string, score int, reason string)
	reminderAfter   time.Duration

	localDB  *db.LocalDB
	recorder *event.Recorder
}

type pendingSuggest struct {
	suggestedAt   time.Time
	agentName     string
	reminderSent  bool
}

type PollerConfig struct {
	Client         *Client
	BoardID        int
	Interval       time.Duration
	OnNewTask      func(Issue)
	OnAssigned     func(Issue)
	OnSuggestAgent func(Issue) (agentName string, score int, reason string)
	ReminderAfter  time.Duration

	// LocalDB + Recorder enable iteration detection. When either is nil,
	// the poller skips iteration tracking — keeps the existing assignment
	// flow unchanged for callers that don't need this feature yet.
	LocalDB  *db.LocalDB
	Recorder *event.Recorder
}

func NewPoller(cfg PollerConfig) *Poller {
	interval := cfg.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	reminderAfter := cfg.ReminderAfter
	if reminderAfter == 0 {
		reminderAfter = 2 * time.Hour
	}

	return &Poller{
		client:          cfg.Client,
		boardID:         cfg.BoardID,
		interval:        interval,
		lastIssueSet:    make(map[string]bool),
		pendingSuggests: make(map[string]pendingSuggest),
		onNewTask:       cfg.OnNewTask,
		onAssigned:      cfg.OnAssigned,
		onSuggestAgent:  cfg.OnSuggestAgent,
		reminderAfter:   reminderAfter,
		localDB:         cfg.LocalDB,
		recorder:        cfg.Recorder,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	slog.Info("jira poller started", "board_id", p.boardID, "interval", p.interval)

	if err := p.Poll(ctx); err != nil {
		slog.Error("initial poll failed", "error", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("jira poller stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := p.Poll(ctx); err != nil {
				slog.Error("poll failed", "error", err)
			}
		}
	}
}

func (p *Poller) Poll(ctx context.Context) error {
	sprint, err := p.client.GetActiveSprint(p.boardID)
	if err != nil {
		return err
	}
	if sprint == nil {
		slog.Debug("no active sprint")
		return nil
	}

	issues, err := p.client.GetSprintIssues(sprint.ID)
	if err != nil {
		return err
	}

	currentSet := make(map[string]bool)
	for _, issue := range issues {
		currentSet[issue.Key] = true

		// New task detected
		if !p.lastIssueSet[issue.Key] {
			slog.Info("new task detected", "key", issue.Key, "summary", issue.Summary)

			links, err := p.client.GetRemoteLinks(issue.Key)
			if err != nil {
				slog.Warn("failed to get remote links", "key", issue.Key, "error", err)
			} else {
				issue.ConfluenceLinks = ExtractConfluenceLinks(links)
			}

			if p.onNewTask != nil {
				p.onNewTask(issue)
			}

			// Suggest agent for new task
			if p.onSuggestAgent != nil && !issue.IsAssigned() {
				agentName, score, reason := p.onSuggestAgent(issue)
				if agentName != "" {
					p.postSuggestionComment(issue.Key, agentName, score, reason)
					p.pendingSuggests[issue.Key] = pendingSuggest{
						suggestedAt: time.Now(),
						agentName:   agentName,
					}
				}
			}
		}

		// Check for confirmation (agent assigned)
		if issue.IsAssigned() && !issue.IsTracked() {
			slog.Info("task assigned", "key", issue.Key, "agent", issue.AgentName)

			// Remove from pending
			delete(p.pendingSuggests, issue.Key)

			if err := p.client.AddLabel(issue.Key, "dandori-tracked"); err != nil {
				slog.Warn("failed to add tracking label", "key", issue.Key, "error", err)
			}

			if p.onAssigned != nil {
				p.onAssigned(issue)
			}
		}

		// Check for reminder on pending suggestions
		if pending, ok := p.pendingSuggests[issue.Key]; ok {
			if !pending.reminderSent && time.Since(pending.suggestedAt) > p.reminderAfter {
				p.postReminderComment(issue.Key, pending.agentName)
				pending.reminderSent = true
				p.pendingSuggests[issue.Key] = pending
			}
		}
	}

	p.lastIssueSet = currentSet

	p.detectIterations(issues)
	return nil
}

// detectIterations walks current sprint issues and emits task.iteration.start
// events for any that have regressed from a prior completed run. Failures here
// must NEVER break the poll cycle — log and continue.
func (p *Poller) detectIterations(issues []Issue) {
	if p.localDB == nil || p.recorder == nil {
		return
	}
	for _, issue := range issues {
		lastRun, err := p.localDB.LatestRunForIssue(issue.Key)
		if err != nil {
			slog.Warn("iteration: latest run lookup failed", "key", issue.Key, "error", err)
			continue
		}
		if lastRun == nil {
			continue
		}
		existing, err := p.localDB.IterationEventsForIssue(issue.Key)
		if err != nil {
			slog.Warn("iteration: events lookup failed", "key", issue.Key, "error", err)
			continue
		}
		evt, err := DetectIteration(&issue,
			&PriorRun{
				RunID:                    lastRun.ID,
				Status:                   lastRun.Status,
				JiraStatusAtCompletion:   lastRun.JiraStatusAtCompletion,
				JiraCategoryAtCompletion: lastRun.JiraCategoryAtCompletion,
				EndedAt:                  lastRun.EndedAt,
			},
			toIterationEvents(existing),
		)
		if err != nil || evt == nil {
			continue
		}
		if err := p.recorder.RecordEvent(lastRun.ID, model.LayerSemantic, "task.iteration.start", evt.Payload()); err != nil {
			slog.Warn("iteration: record event failed", "key", issue.Key, "error", err)
			continue
		}
		slog.Info("iteration detected", "key", issue.Key, "round", evt.Round, "prev_run", evt.PrevRunID)
	}
}

func toIterationEvents(rows []db.IterationEventRow) []IterationEvent {
	out := make([]IterationEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, IterationEvent{
			Round:          r.Round,
			IssueKey:       r.IssueKey,
			TransitionedAt: r.TransitionedAt,
		})
	}
	return out
}

func (p *Poller) postSuggestionComment(issueKey, agentName string, score int, reason string) {
	comment := "🤖 *Agent Suggestion*\n\n" +
		"*Suggested agent:* " + agentName + " (" + itoa(score) + "%)\n" +
		"*Reason:* " + reason + "\n\n" +
		"To confirm: set `dandori-agent` field to `" + agentName + "`"

	if err := p.client.AddComment(issueKey, comment); err != nil {
		slog.Warn("failed to post suggestion comment", "key", issueKey, "error", err)
	} else {
		slog.Info("posted suggestion", "key", issueKey, "agent", agentName, "score", score)
	}
}

func (p *Poller) postReminderComment(issueKey, agentName string) {
	comment := "⏰ *Reminder*: Agent suggestion pending confirmation.\n\n" +
		"Suggested agent: *" + agentName + "*\n\n" +
		"Please set `dandori-agent` field to confirm or assign a different agent."

	if err := p.client.AddComment(issueKey, comment); err != nil {
		slog.Warn("failed to post reminder", "key", issueKey, "error", err)
	} else {
		slog.Info("posted reminder", "key", issueKey)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func (p *Poller) GetLastIssueSet() map[string]bool {
	return p.lastIssueSet
}
