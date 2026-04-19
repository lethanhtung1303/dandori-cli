package quality

import (
	"testing"
)

func TestParseGoTestJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTotal   int
		wantPassed  int
		wantFailed  int
		wantSkipped int
	}{
		{
			name:        "empty output",
			input:       "",
			wantTotal:   0,
			wantPassed:  0,
			wantFailed:  0,
			wantSkipped: 0,
		},
		{
			name: "single passing test",
			input: `{"Time":"2026-04-19T10:00:00Z","Action":"run","Package":"example","Test":"TestFoo"}
{"Time":"2026-04-19T10:00:01Z","Action":"pass","Package":"example","Test":"TestFoo","Elapsed":0.5}`,
			wantTotal:   1,
			wantPassed:  1,
			wantFailed:  0,
			wantSkipped: 0,
		},
		{
			name: "single failing test",
			input: `{"Time":"2026-04-19T10:00:00Z","Action":"run","Package":"example","Test":"TestBar"}
{"Time":"2026-04-19T10:00:01Z","Action":"fail","Package":"example","Test":"TestBar","Elapsed":0.3}`,
			wantTotal:   1,
			wantPassed:  0,
			wantFailed:  1,
			wantSkipped: 0,
		},
		{
			name: "mixed results",
			input: `{"Action":"run","Package":"pkg","Test":"TestA"}
{"Action":"pass","Package":"pkg","Test":"TestA"}
{"Action":"run","Package":"pkg","Test":"TestB"}
{"Action":"fail","Package":"pkg","Test":"TestB"}
{"Action":"run","Package":"pkg","Test":"TestC"}
{"Action":"pass","Package":"pkg","Test":"TestC"}
{"Action":"run","Package":"pkg","Test":"TestD"}
{"Action":"skip","Package":"pkg","Test":"TestD"}`,
			wantTotal:   4,
			wantPassed:  2,
			wantFailed:  1,
			wantSkipped: 1,
		},
		{
			name: "package-level events ignored",
			input: `{"Action":"start","Package":"pkg"}
{"Action":"run","Package":"pkg","Test":"TestOne"}
{"Action":"pass","Package":"pkg","Test":"TestOne"}
{"Action":"pass","Package":"pkg"}`,
			wantTotal:   1,
			wantPassed:  1,
			wantFailed:  0,
			wantSkipped: 0,
		},
		{
			name: "subtests counted separately",
			input: `{"Action":"run","Package":"pkg","Test":"TestParent"}
{"Action":"run","Package":"pkg","Test":"TestParent/SubA"}
{"Action":"pass","Package":"pkg","Test":"TestParent/SubA"}
{"Action":"run","Package":"pkg","Test":"TestParent/SubB"}
{"Action":"fail","Package":"pkg","Test":"TestParent/SubB"}
{"Action":"fail","Package":"pkg","Test":"TestParent"}`,
			wantTotal:   3,
			wantPassed:  1,
			wantFailed:  2,
			wantSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, passed, failed, skipped, err := ParseGoTestJSON([]byte(tt.input))
			if err != nil {
				t.Errorf("ParseGoTestJSON() error = %v", err)
				return
			}
			if total != tt.wantTotal {
				t.Errorf("total = %v, want %v", total, tt.wantTotal)
			}
			if passed != tt.wantPassed {
				t.Errorf("passed = %v, want %v", passed, tt.wantPassed)
			}
			if failed != tt.wantFailed {
				t.Errorf("failed = %v, want %v", failed, tt.wantFailed)
			}
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}

func TestParseTestSummary(t *testing.T) {
	input := `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- FAIL: TestBar (0.01s)
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
FAIL`

	total, passed, failed, _ := ParseTestSummary(input)
	if total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
	if passed != 2 {
		t.Errorf("passed = %v, want 2", passed)
	}
	if failed != 1 {
		t.Errorf("failed = %v, want 1", failed)
	}
}
