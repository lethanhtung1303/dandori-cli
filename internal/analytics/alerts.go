package analytics

// RunStat is the minimal per-row input DetectAlerts needs.
// Kept intentionally thin — built from DB rows by the caller.
type RunStat struct {
	Engineer  string
	Agent     string
	Cost      float64
	ACPercent float64 // Acceptance-Criteria completion, 0..100
}

// Alert is one threshold breach message.
// Kind/DrilldownURL are populated for dashboard surfacing; CLI ignores them.
type Alert struct {
	Kind         string `json:"kind"` // "cost_multiple" or "ac_dip"
	Severity     string `json:"severity"` // "warn" for now
	Message      string `json:"message"`
	DrilldownURL string `json:"drilldown_url,omitempty"`
}

// Thresholds control when alerts fire. Zero value means "disabled".
type Thresholds struct {
	ACMinPercent float64 // engineer flagged if ACPercent < ACMinPercent (and > 0)
	CostMultiple float64 // agent flagged if cost > CostMultiple × median of other agents
}

func DefaultThresholds() Thresholds {
	return Thresholds{ACMinPercent: 80, CostMultiple: 3.0}
}

// DetectAlerts applies thresholds to the provided stats.
// Pure function — no DB access, safe to unit-test.
func DetectAlerts(runs []RunStat, th Thresholds) []Alert {
	var alerts []Alert
	if len(runs) == 0 {
		return alerts
	}

	if th.ACMinPercent > 0 {
		seen := map[string]bool{}
		for _, r := range runs {
			if r.Engineer == "" || seen[r.Engineer] {
				continue
			}
			if r.ACPercent > 0 && r.ACPercent < th.ACMinPercent {
				alerts = append(alerts, Alert{
					Kind:         "ac_dip",
					Severity:     "warn",
					Message:      r.Engineer + ": AC completion below baseline",
					DrilldownURL: "?role=engineer&id=" + r.Engineer,
				})
				seen[r.Engineer] = true
			}
		}
	}

	if th.CostMultiple > 0 {
		agentTotals := map[string]float64{}
		for _, r := range runs {
			if r.Agent == "" {
				continue
			}
			agentTotals[r.Agent] += r.Cost
		}
		if len(agentTotals) >= 2 {
			var others []float64
			for _, c := range agentTotals {
				others = append(others, c)
			}
			for name, cost := range agentTotals {
				var sumOthers float64
				var n int
				for other, c := range agentTotals {
					if other == name {
						continue
					}
					sumOthers += c
					n++
				}
				if n == 0 {
					continue
				}
				baseline := sumOthers / float64(n)
				if baseline > 0 && cost >= th.CostMultiple*baseline {
					alerts = append(alerts, Alert{
						Kind:         "cost_multiple",
						Severity:     "warn",
						Message:      name + ": cost " + formatMultiple(cost/baseline) + "× baseline",
						DrilldownURL: "?role=agent&id=" + name,
					})
				}
			}
		}
	}

	return alerts
}

func formatMultiple(x float64) string {
	// one decimal place, no trailing zero: 3.0 → "3", 3.2 → "3.2"
	i := int(x)
	frac := int((x - float64(i)) * 10)
	if frac == 0 {
		return itoa(i)
	}
	return itoa(i) + "." + itoa(frac)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
