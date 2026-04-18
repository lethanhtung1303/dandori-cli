package jira

import (
	"strings"
	"time"
)

type Board struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location struct {
		ProjectKey string `json:"projectKey"`
	} `json:"location"`
}

type Sprint struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	Goal      string    `json:"goal"`
}

type Issue struct {
	Key         string
	Summary     string
	Description string
	IssueType   string
	Priority    string
	Status      string
	SprintID    int
	SprintName  string
	Assignee    string
	Labels      []string
	StoryPoints float64
	AgentName   string
	EpicKey     string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	ConfluenceLinks []ConfluenceLink
}

type ConfluenceLink struct {
	PageID   string
	Title    string
	URL      string
	SpaceKey string
}

type RemoteLink struct {
	ID     int `json:"id"`
	Object struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Icon  struct {
			URL16x16 string `json:"url16x16"`
		} `json:"icon"`
	} `json:"object"`
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name string `json:"name"`
	} `json:"to"`
}

type boardsResponse struct {
	Values []Board `json:"values"`
}

type sprintsResponse struct {
	Values []Sprint `json:"values"`
}

type issuesResponse struct {
	Issues []issueResponse `json:"issues"`
}

type issueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description string `json:"description"`
		IssueType   struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		Status struct {
			Name string `json:"name"`
		} `json:"status"`
		Labels   []string `json:"labels"`
		Assignee struct {
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
		Sprint struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"sprint"`
		Epic struct {
			Key string `json:"key"`
		} `json:"epic"`
		StoryPoints  float64   `json:"customfield_10020"`
		AgentName    string    `json:"customfield_10100"`
		Created      time.Time `json:"created"`
		Updated      time.Time `json:"updated"`
	} `json:"fields"`
}

func parseIssue(resp *issueResponse) *Issue {
	return &Issue{
		Key:         resp.Key,
		Summary:     resp.Fields.Summary,
		Description: resp.Fields.Description,
		IssueType:   resp.Fields.IssueType.Name,
		Priority:    resp.Fields.Priority.Name,
		Status:      resp.Fields.Status.Name,
		SprintID:    resp.Fields.Sprint.ID,
		SprintName:  resp.Fields.Sprint.Name,
		Assignee:    resp.Fields.Assignee.DisplayName,
		Labels:      resp.Fields.Labels,
		StoryPoints: resp.Fields.StoryPoints,
		AgentName:   resp.Fields.AgentName,
		EpicKey:     resp.Fields.Epic.Key,
		CreatedAt:   resp.Fields.Created,
		UpdatedAt:   resp.Fields.Updated,
	}
}

func (i *Issue) HasLabel(label string) bool {
	for _, l := range i.Labels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

func (i *Issue) IsTracked() bool {
	return i.HasLabel("dandori-tracked")
}

func (i *Issue) IsAssigned() bool {
	return i.AgentName != ""
}

func ExtractConfluenceLinks(links []RemoteLink) []ConfluenceLink {
	var result []ConfluenceLink
	for _, link := range links {
		if isConfluenceURL(link.Object.URL) {
			cl := ConfluenceLink{
				URL:   link.Object.URL,
				Title: link.Object.Title,
			}
			cl.PageID, cl.SpaceKey = parseConfluenceURL(link.Object.URL)
			result = append(result, cl)
		}
	}
	return result
}

func isConfluenceURL(url string) bool {
	return strings.Contains(url, "/wiki/") ||
		strings.Contains(url, "confluence") ||
		strings.Contains(url, "/pages/")
}

func parseConfluenceURL(url string) (pageID, spaceKey string) {
	if idx := strings.Index(url, "/pages/viewpage.action?pageId="); idx != -1 {
		pageID = url[idx+len("/pages/viewpage.action?pageId="):]
		if ampIdx := strings.Index(pageID, "&"); ampIdx != -1 {
			pageID = pageID[:ampIdx]
		}
	}

	if idx := strings.Index(url, "/spaces/"); idx != -1 {
		rest := url[idx+len("/spaces/"):]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			spaceKey = rest[:slashIdx]
		} else {
			spaceKey = rest
		}
	}

	return
}
