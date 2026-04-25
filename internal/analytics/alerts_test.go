package analytics

import (
	"strings"
	"testing"
)

func TestAlerts_ACCompletionBelowThreshold(t *testing.T) {
	runs := []RunStat{{Engineer: "Carol", ACPercent: 64}}
	alerts := DetectAlerts(runs, DefaultThresholds())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d (%+v)", len(alerts), alerts)
	}
	if !strings.Contains(alerts[0].Message, "Carol") {
		t.Errorf("alert not about Carol: %+v", alerts[0])
	}
}

func TestAlerts_CostMultipleBaseline(t *testing.T) {
	runs := []RunStat{
		{Agent: "alpha", Cost: 10},
		{Agent: "alpha", Cost: 2},
		{Agent: "beta", Cost: 11},
		{Agent: "RefactorBot", Cost: 35},
	}
	alerts := DetectAlerts(runs, Thresholds{CostMultiple: 3.0})
	found := false
	for _, a := range alerts {
		if strings.Contains(a.Message, "RefactorBot") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected RefactorBot alert, got %+v", alerts)
	}
}

func TestAlerts_NoFalsePositiveEmptyData(t *testing.T) {
	alerts := DetectAlerts(nil, DefaultThresholds())
	if len(alerts) != 0 {
		t.Errorf("false positives on empty input: %+v", alerts)
	}
}

func TestAlerts_ACThresholdZeroDisabled(t *testing.T) {
	runs := []RunStat{{Engineer: "Carol", ACPercent: 10}}
	alerts := DetectAlerts(runs, Thresholds{ACMinPercent: 0})
	if len(alerts) != 0 {
		t.Errorf("expected no alerts when ACMinPercent=0, got %+v", alerts)
	}
}
