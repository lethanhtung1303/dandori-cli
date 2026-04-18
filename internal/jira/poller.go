package jira

import (
	"context"
	"log/slog"
	"time"
)

type Poller struct {
	client       *Client
	boardID      int
	interval     time.Duration
	lastIssueSet map[string]bool
	onNewTask    func(Issue)
	onAssigned   func(Issue)
}

type PollerConfig struct {
	Client     *Client
	BoardID    int
	Interval   time.Duration
	OnNewTask  func(Issue)
	OnAssigned func(Issue)
}

func NewPoller(cfg PollerConfig) *Poller {
	interval := cfg.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	return &Poller{
		client:       cfg.Client,
		boardID:      cfg.BoardID,
		interval:     interval,
		lastIssueSet: make(map[string]bool),
		onNewTask:    cfg.OnNewTask,
		onAssigned:   cfg.OnAssigned,
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
		}

		if issue.IsAssigned() && !issue.IsTracked() {
			slog.Info("task assigned", "key", issue.Key, "agent", issue.AgentName)

			if err := p.client.AddLabel(issue.Key, "dandori-tracked"); err != nil {
				slog.Warn("failed to add tracking label", "key", issue.Key, "error", err)
			}

			if p.onAssigned != nil {
				p.onAssigned(issue)
			}
		}
	}

	p.lastIssueSet = currentSet
	return nil
}

func (p *Poller) GetLastIssueSet() map[string]bool {
	return p.lastIssueSet
}
