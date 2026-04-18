package jira

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

type CommentData struct {
	IssueKey      string
	IssueType     string
	Labels        string
	AgentName     string
	Capabilities  string
	Score         int
	AgentList     string
	RunID         string
	WorkstationID string
	StartedAt     string
	Duration      string
	InputTokens   int
	OutputTokens  int
	CostUSD       float64
	ExitCode      int
	GitHeadBefore string
	GitHeadAfter  string
	LastError     string
}

const suggestionTemplate = `🤖 *Dandori Agent Suggestion*

Based on task type ({{.IssueType}}) and labels ({{.Labels}}):
- Recommended agent: *{{.AgentName}}* ({{.Capabilities}})
- Confidence: {{.Score}}%

To confirm: set field ` + "`dandori-agent`" + ` to ` + "`{{.AgentName}}`" + `
To assign different agent: set field to any registered agent name.

_Available agents: {{.AgentList}}_`

const runStartedTemplate = `🟢 Agent *{{.AgentName}}* started working on this task.
- Run ID: {{.RunID}}
- Workstation: {{.WorkstationID}}
- Started: {{.StartedAt}}`

const runCompletedTemplate = `✅ Agent *{{.AgentName}}* completed.
- Duration: {{.Duration}}
- Tokens: {{.InputTokens}} in / {{.OutputTokens}} out
- Cost: ${{printf "%.4f" .CostUSD}}
- Exit code: {{.ExitCode}}
- Git: {{.GitHeadBefore}} → {{.GitHeadAfter}}`

const runFailedTemplate = `❌ Agent *{{.AgentName}}* failed.
- Duration: {{.Duration}}
- Exit code: {{.ExitCode}}
- Error: {{.LastError}}`

func RenderSuggestion(data CommentData) (string, error) {
	return renderTemplate("suggestion", suggestionTemplate, data)
}

func RenderRunStarted(data CommentData) (string, error) {
	return renderTemplate("run_started", runStartedTemplate, data)
}

func RenderRunCompleted(data CommentData) (string, error) {
	return renderTemplate("run_completed", runCompletedTemplate, data)
}

func RenderRunFailed(data CommentData) (string, error) {
	return renderTemplate("run_failed", runFailedTemplate, data)
}

func renderTemplate(name, tmplStr string, data CommentData) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func (c *Client) AddComment(issueKey, body string) error {
	payload := map[string]string{"body": body}
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment", issueKey)
	return c.post(path, payload, nil)
}

func (c *Client) PostSuggestion(issueKey string, data CommentData) error {
	body, err := RenderSuggestion(data)
	if err != nil {
		return err
	}
	return c.AddComment(issueKey, body)
}

func (c *Client) PostRunStarted(issueKey, agentName, runID, workstationID string, startedAt time.Time) error {
	data := CommentData{
		AgentName:     agentName,
		RunID:         runID,
		WorkstationID: workstationID,
		StartedAt:     startedAt.Format(time.RFC3339),
	}
	body, err := RenderRunStarted(data)
	if err != nil {
		return err
	}
	return c.AddComment(issueKey, body)
}

func (c *Client) PostRunCompleted(issueKey string, data CommentData) error {
	body, err := RenderRunCompleted(data)
	if err != nil {
		return err
	}
	return c.AddComment(issueKey, body)
}

func (c *Client) PostRunFailed(issueKey string, data CommentData) error {
	body, err := RenderRunFailed(data)
	if err != nil {
		return err
	}
	return c.AddComment(issueKey, body)
}

func (c *Client) AddLabel(issueKey, label string) error {
	issue, err := c.GetIssue(issueKey)
	if err != nil {
		return err
	}

	for _, l := range issue.Labels {
		if strings.EqualFold(l, label) {
			return nil
		}
	}

	labels := append(issue.Labels, label)
	payload := map[string]any{
		"fields": map[string]any{
			"labels": labels,
		},
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
	return c.put(path, payload)
}

func (c *Client) SetAgentField(issueKey, agentName, fieldID string) error {
	payload := map[string]any{
		"fields": map[string]any{
			fieldID: agentName,
		},
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
	return c.put(path, payload)
}
