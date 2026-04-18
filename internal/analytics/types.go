package analytics

import "time"

type Filters struct {
	From      *time.Time
	To        *time.Time
	Agent     string
	Team      string
	Project   string
	SprintID  string
	IssueType string
}

type AgentStat struct {
	AgentName   string    `json:"agent_name"`
	Day         time.Time `json:"day,omitempty"`
	RunCount    int       `json:"run_count"`
	SuccessRate float64   `json:"success_rate"`
	TotalCost   float64   `json:"total_cost"`
	AvgCost     float64   `json:"avg_cost"`
	AvgDuration float64   `json:"avg_duration"`
}

type AgentComparison struct {
	AgentName       string  `json:"agent_name"`
	RunCount        int     `json:"run_count"`
	SuccessRate     float64 `json:"success_rate"`
	TotalCost       float64 `json:"total_cost"`
	AvgCost         float64 `json:"avg_cost"`
	AvgDuration     float64 `json:"avg_duration"`
	PointsCompleted float64 `json:"points_completed,omitempty"`
}

type TaskTypeStat struct {
	IssueType   string  `json:"issue_type"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
	AvgCost     float64 `json:"avg_cost"`
	AvgDuration float64 `json:"avg_duration"`
	TotalCost   float64 `json:"total_cost"`
}

type CostGroup struct {
	Group    string  `json:"group"`
	Cost     float64 `json:"cost"`
	RunCount int     `json:"run_count"`
	Tokens   int     `json:"tokens"`
}

type TrendPoint struct {
	PeriodStart time.Time `json:"period_start"`
	Cost        float64   `json:"cost"`
	PrevCost    float64   `json:"prev_cost"`
	ChangePct   float64   `json:"change_pct"`
}

type SprintSummary struct {
	SprintID        string  `json:"sprint_id"`
	SprintName      string  `json:"sprint_name"`
	TaskCount       int     `json:"task_count"`
	CompletedCount  int     `json:"completed_count"`
	AgentCount      int     `json:"agent_count"`
	TotalRuns       int     `json:"total_runs"`
	TotalCost       float64 `json:"total_cost"`
	PointsCompleted float64 `json:"points_completed"`
	PointsPerDollar float64 `json:"points_per_dollar"`
}

type TaskCost struct {
	IssueKey  string        `json:"issue_key"`
	TotalCost float64       `json:"total_cost"`
	Runs      []TaskCostRun `json:"runs"`
}

type TaskCostRun struct {
	RunID    string  `json:"run_id"`
	Agent    string  `json:"agent"`
	Cost     float64 `json:"cost"`
	Duration float64 `json:"duration"`
	Status   string  `json:"status"`
}
