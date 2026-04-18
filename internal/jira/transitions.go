package jira

import (
	"fmt"
	"strings"
)

type StatusMapping struct {
	Running string
	Done    string
	Error   string
}

var DefaultStatusMapping = StatusMapping{
	Running: "In Progress",
	Done:    "Done",
	Error:   "To Do",
}

func (c *Client) GetTransitions(issueKey string) ([]Transition, error) {
	var resp struct {
		Transitions []Transition `json:"transitions"`
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueKey)
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}

	return resp.Transitions, nil
}

func (c *Client) TransitionTo(issueKey, targetStatus string) error {
	transitions, err := c.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("get transitions: %w", err)
	}

	var transitionID string
	for _, t := range transitions {
		if strings.EqualFold(t.To.Name, targetStatus) ||
			strings.EqualFold(t.Name, targetStatus) {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return fmt.Errorf("no transition found to status %q (available: %v)", targetStatus, getTransitionNames(transitions))
	}

	payload := map[string]any{
		"transition": map[string]string{
			"id": transitionID,
		},
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueKey)
	return c.post(path, payload, nil)
}

func (c *Client) TransitionToRunning(issueKey string, mapping StatusMapping) error {
	return c.TransitionTo(issueKey, mapping.Running)
}

func (c *Client) TransitionToDone(issueKey string, mapping StatusMapping) error {
	return c.TransitionTo(issueKey, mapping.Done)
}

func (c *Client) TransitionToError(issueKey string, mapping StatusMapping) error {
	return c.TransitionTo(issueKey, mapping.Error)
}

func getTransitionNames(transitions []Transition) []string {
	names := make([]string, len(transitions))
	for i, t := range transitions {
		names[i] = t.Name + " → " + t.To.Name
	}
	return names
}
