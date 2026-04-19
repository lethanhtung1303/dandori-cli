package quality

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// GoTestEvent represents a single event from go test -json output
type GoTestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// ParseGoTestJSON parses go test -json output (stream of JSON objects)
// Returns (total, passed, failed, skipped, error)
func ParseGoTestJSON(output []byte) (int, int, int, int, error) {
	if len(output) == 0 {
		return 0, 0, 0, 0, nil
	}

	// Track test results by name (package/TestName)
	testResults := make(map[string]string)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var event GoTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip non-JSON lines
		}

		// Only track test-level results (not package-level)
		if event.Test == "" {
			continue
		}

		testKey := event.Package + "/" + event.Test

		// Update result based on action
		switch event.Action {
		case "pass":
			testResults[testKey] = "pass"
		case "fail":
			testResults[testKey] = "fail"
		case "skip":
			testResults[testKey] = "skip"
		}
	}

	// Count results
	total := len(testResults)
	passed := 0
	failed := 0
	skipped := 0

	for _, result := range testResults {
		switch result {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}

	return total, passed, failed, skipped, nil
}

// ParseTestSummary parses simple test output (fallback)
// Looks for patterns like "PASS", "FAIL", "ok", "FAIL"
func ParseTestSummary(output string) (int, int, int, int) {
	lines := strings.Split(output, "\n")
	passed := 0
	failed := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- PASS:") {
			passed++
		} else if strings.HasPrefix(line, "--- FAIL:") {
			failed++
		}
	}

	total := passed + failed
	return total, passed, failed, 0
}
