package quality

import "time"

// Snapshot captures lint/test state at a point in time
type Snapshot struct {
	LintErrors   int
	LintWarnings int
	TestsTotal   int
	TestsPassed  int
	TestsFailed  int
	TestsSkipped int
	CapturedAt   time.Time
	Error        string // Non-empty if capture failed
}

// Metrics stores quality comparison between before/after snapshots
type Metrics struct {
	RunID string

	// Before
	LintErrorsBefore   int
	LintWarningsBefore int
	TestsTotalBefore   int
	TestsPassedBefore  int
	TestsFailedBefore  int

	// After
	LintErrorsAfter   int
	LintWarningsAfter int
	TestsTotalAfter   int
	TestsPassedAfter  int
	TestsFailedAfter  int

	// Computed deltas
	LintDelta  int // Negative = improvement (fewer errors)
	TestsDelta int // Positive = improvement (more passing)

	CreatedAt time.Time
}

// ComputeMetrics calculates deltas between before and after snapshots
func ComputeMetrics(runID string, before, after *Snapshot) *Metrics {
	m := &Metrics{
		RunID:     runID,
		CreatedAt: time.Now(),
	}

	if before != nil {
		m.LintErrorsBefore = before.LintErrors
		m.LintWarningsBefore = before.LintWarnings
		m.TestsTotalBefore = before.TestsTotal
		m.TestsPassedBefore = before.TestsPassed
		m.TestsFailedBefore = before.TestsFailed
	}

	if after != nil {
		m.LintErrorsAfter = after.LintErrors
		m.LintWarningsAfter = after.LintWarnings
		m.TestsTotalAfter = after.TestsTotal
		m.TestsPassedAfter = after.TestsPassed
		m.TestsFailedAfter = after.TestsFailed
	}

	// Compute deltas
	m.LintDelta = m.LintErrorsAfter - m.LintErrorsBefore   // Negative = improvement
	m.TestsDelta = m.TestsPassedAfter - m.TestsPassedBefore // Positive = improvement

	return m
}

// IsImproved returns true if quality improved overall
func (m *Metrics) IsImproved() bool {
	// Fewer lint errors OR more passing tests
	return m.LintDelta < 0 || m.TestsDelta > 0
}

// Summary returns a human-readable summary
func (m *Metrics) Summary() string {
	if m.LintErrorsBefore == 0 && m.LintErrorsAfter == 0 &&
		m.TestsPassedBefore == 0 && m.TestsPassedAfter == 0 {
		return "No quality data"
	}

	return ""
}
