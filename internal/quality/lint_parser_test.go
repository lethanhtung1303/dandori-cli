package quality

import (
	"testing"
)

func TestParseGolangciLint(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantErrors   int
		wantWarnings int
		wantErr      bool
	}{
		{
			name:         "empty output",
			input:        "",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "null issues",
			input:        `{"Issues": null}`,
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "empty issues array",
			input:        `{"Issues": []}`,
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name: "single error",
			input: `{
				"Issues": [
					{"FromLinter": "govet", "Text": "unreachable code", "Severity": "error"}
				]
			}`,
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name: "single warning",
			input: `{
				"Issues": [
					{"FromLinter": "golint", "Text": "exported func should have comment", "Severity": "warning"}
				]
			}`,
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name: "mixed errors and warnings",
			input: `{
				"Issues": [
					{"FromLinter": "govet", "Text": "unreachable code", "Severity": "error"},
					{"FromLinter": "golint", "Text": "missing comment", "Severity": "warning"},
					{"FromLinter": "errcheck", "Text": "error return value not checked", "Severity": "error"},
					{"FromLinter": "staticcheck", "Text": "deprecated function", "Severity": "warning"}
				]
			}`,
			wantErrors:   2,
			wantWarnings: 2,
		},
		{
			name: "empty severity defaults to error",
			input: `{
				"Issues": [
					{"FromLinter": "govet", "Text": "unreachable code", "Severity": ""}
				]
			}`,
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name: "unknown severity treated as warning",
			input: `{
				"Issues": [
					{"FromLinter": "custom", "Text": "something", "Severity": "info"}
				]
			}`,
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name: "fallback to line parsing for non-JSON",
			input: `main.go:10:5: error: undefined variable
main.go:15:3: warning: unused import`,
			wantErrors:   1,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, warnings, err := ParseGolangciLint([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGolangciLint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if errors != tt.wantErrors {
				t.Errorf("ParseGolangciLint() errors = %v, want %v", errors, tt.wantErrors)
			}
			if warnings != tt.wantWarnings {
				t.Errorf("ParseGolangciLint() warnings = %v, want %v", warnings, tt.wantWarnings)
			}
		})
	}
}
