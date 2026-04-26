package jira

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/event"
)

// stubJiraServer returns a server that exposes a single sprint (id=42) with
// one issue whose status/category can be flipped between calls. Reflects how
// Jira behaves when PO drags Done → In Progress.
func stubJiraServer(t *testing.T, status, category string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/board/1/sprint"):
			w.Write([]byte(`{"values":[{"id":42,"name":"S","state":"active"}]}`))
		case strings.Contains(r.URL.Path, "/sprint/42/issue"):
			body := `{"issues":[{"key":"KEY-1","fields":{
				"summary":"t","status":{"name":"` + status + `","statusCategory":{"key":"` + category + `"}},
				"issuetype":{"name":"Task"},"priority":{"name":"Medium"},"labels":[],
				"assignee":{"displayName":""},"sprint":{"id":42,"name":"S"},"epic":{"key":""},
				"created":"2026-04-20T08:00:00.000Z","updated":"2026-04-25T10:00:00.000Z"
			}}]}`
			w.Write([]byte(body))
		case strings.Contains(r.URL.Path, "/remotelink"):
			w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func openTestDBForPoller(t *testing.T) *db.LocalDB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seedDoneRun(t *testing.T, d *db.LocalDB, runID, issueKey string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_type, user, workstation_id, started_at, status)
		VALUES (?, ?, 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, issueKey, time.Now().Add(-time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func countIterationEvents(t *testing.T, d *db.LocalDB, issueKey string) int {
	t.Helper()
	rows, err := d.IterationEventsForIssue(issueKey)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return len(rows)
}

// TestPoller_DetectsIterationOnRegression: KEY-1 was Done (prior run done),
// poller sees status now In Progress → emits exactly one iteration event.
func TestPoller_DetectsIterationOnRegression(t *testing.T) {
	srv := stubJiraServer(t, "In Progress", "indeterminate")
	d := openTestDBForPoller(t)
	rec := event.NewRecorder(d)
	seedDoneRun(t, d, "prev-run-1", "KEY-1")

	client := NewClient(ClientConfig{BaseURL: srv.URL, User: "u", Token: "t", IsCloud: true})
	p := NewPoller(PollerConfig{
		Client:   client,
		BoardID:  1,
		LocalDB:  d,
		Recorder: rec,
	})

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	if got := countIterationEvents(t, d, "KEY-1"); got != 1 {
		t.Errorf("after 1 cycle: %d events, want 1", got)
	}
}

// TestPoller_DedupesIterationAcrossCycles: same transition should not emit
// twice when poller runs back-to-back.
func TestPoller_DedupesIterationAcrossCycles(t *testing.T) {
	srv := stubJiraServer(t, "In Progress", "indeterminate")
	d := openTestDBForPoller(t)
	rec := event.NewRecorder(d)
	seedDoneRun(t, d, "prev-run-1", "KEY-1")

	client := NewClient(ClientConfig{BaseURL: srv.URL, User: "u", Token: "t", IsCloud: true})
	p := NewPoller(PollerConfig{Client: client, BoardID: 1, LocalDB: d, Recorder: rec})

	for i := 0; i < 3; i++ {
		if err := p.Poll(context.Background()); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}

	if got := countIterationEvents(t, d, "KEY-1"); got != 1 {
		t.Errorf("after 3 cycles: %d events, want 1 (dedupe)", got)
	}
}

// TestPoller_NoIterationWhenStillDone: status is still done — no event.
func TestPoller_NoIterationWhenStillDone(t *testing.T) {
	srv := stubJiraServer(t, "Done", "done")
	d := openTestDBForPoller(t)
	rec := event.NewRecorder(d)
	seedDoneRun(t, d, "prev-run-1", "KEY-1")

	client := NewClient(ClientConfig{BaseURL: srv.URL, User: "u", Token: "t", IsCloud: true})
	p := NewPoller(PollerConfig{Client: client, BoardID: 1, LocalDB: d, Recorder: rec})

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if got := countIterationEvents(t, d, "KEY-1"); got != 0 {
		t.Errorf("got %d events, want 0", got)
	}
}

// TestPoller_NoDB_NoOp: when LocalDB/Recorder absent, poller still runs.
func TestPoller_NoDB_NoOp(t *testing.T) {
	srv := stubJiraServer(t, "In Progress", "indeterminate")
	client := NewClient(ClientConfig{BaseURL: srv.URL, User: "u", Token: "t", IsCloud: true})
	p := NewPoller(PollerConfig{Client: client, BoardID: 1})

	if err := p.Poll(context.Background()); err != nil {
		t.Errorf("poll without db: %v", err)
	}
}
