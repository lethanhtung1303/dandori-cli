package intent

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_ShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestTruncate_ExactBoundary(t *testing.T) {
	s := strings.Repeat("a", 100)
	got := truncate(s, 100)
	if got != s {
		t.Fatalf("expected string unchanged at boundary")
	}
}

func TestTruncate_OversizedAppendsSuffix(t *testing.T) {
	s := strings.Repeat("x", 200)
	got := truncate(s, 50)
	if !strings.HasSuffix(got, truncationSuffix) {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
	if len(got) > 50+len(truncationSuffix) {
		t.Fatalf("result too long: %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// redactSecrets
// ---------------------------------------------------------------------------

func TestRedactSecrets_SkKey(t *testing.T) {
	input := "use sk-proj-abcdefghijklmnopqrst1234567890 to authenticate"
	out := redactSecrets(input)
	if strings.Contains(out, "sk-proj") {
		t.Fatalf("sk- key not redacted: %q", out)
	}
	if !strings.Contains(out, "<redacted>") {
		t.Fatalf("expected <redacted> in output: %q", out)
	}
}

func TestRedactSecrets_BearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig"
	out := redactSecrets(input)
	if strings.Contains(out, "eyJ") {
		t.Fatalf("Bearer token not redacted: %q", out)
	}
}

func TestRedactSecrets_ApiKeyAssignment(t *testing.T) {
	input := `config: api_key="supersecretvalue123"`
	out := redactSecrets(input)
	if strings.Contains(out, "supersecretvalue") {
		t.Fatalf("api_key value not redacted: %q", out)
	}
}

func TestRedactSecrets_CleanText(t *testing.T) {
	input := "Implement the login feature using JWT"
	out := redactSecrets(input)
	if out != input {
		t.Fatalf("clean text should be unchanged, got %q", out)
	}
}

func TestRedactSecrets_ProseKeywordsNotRedacted(t *testing.T) {
	cases := []string{
		"Add password hashing with bcrypt",
		"Reset the user password flow",
		"Generate a new token for the API",
		"This is a secret feature we plan to ship",
		"The api_key feature is documented separately",
	}
	for _, in := range cases {
		out := redactSecrets(in)
		if out != in {
			t.Errorf("prose text incorrectly redacted:\n  in:  %q\n  out: %q", in, out)
		}
	}
}

func TestRedactSecrets_AssignmentFormsStillRedacted(t *testing.T) {
	cases := []struct {
		in       string
		mustHide string
	}{
		{`password=hunter2xyz`, "hunter2xyz"},
		{`token: abc123def456`, "abc123def456"},
		{`API_KEY = "myrealsecret"`, "myrealsecret"},
		{`{"secret":"shouldnotsee"}`, "shouldnotsee"},
		{`{"password": "alsohidden"}`, "alsohidden"},
	}
	for _, tc := range cases {
		out := redactSecrets(tc.in)
		if strings.Contains(out, tc.mustHide) {
			t.Errorf("secret value leaked:\n  in:  %q\n  out: %q", tc.in, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Walk
// ---------------------------------------------------------------------------

func TestWalk_SimpleFile(t *testing.T) {
	var lines []parsedLine
	err := Walk("testdata/simple.jsonl", func(pl parsedLine) {
		lines = append(lines, pl)
	})
	if err != nil {
		t.Fatalf("Walk error: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected lines, got none")
	}
}

func TestWalk_MalformedLines_DoNotPanic(t *testing.T) {
	var lines []parsedLine
	err := Walk("testdata/malformed.jsonl", func(pl parsedLine) {
		lines = append(lines, pl)
	})
	if err != nil {
		t.Fatalf("Walk should not return error on malformed lines, got: %v", err)
	}
	// At least first user + two assistant lines should survive
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 good lines, got %d", len(lines))
	}
}

func TestWalk_NonexistentFile(t *testing.T) {
	err := Walk("testdata/does_not_exist.jsonl", func(parsedLine) {})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Extract — happy path (simple fixture)
// ---------------------------------------------------------------------------

func TestExtract_Simple_FirstUserMsg(t *testing.T) {
	res, err := Extract("testdata/simple.jsonl", "run-test-001", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	want := "Implement the login feature with JWT authentication"
	if res.FirstUserMsg != want {
		t.Fatalf("FirstUserMsg: want %q, got %q", want, res.FirstUserMsg)
	}
}

func TestExtract_Simple_Summary(t *testing.T) {
	res, err := Extract("testdata/simple.jsonl", "run-test-001", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if res.Summary == "" {
		t.Fatal("expected non-empty Summary")
	}
	if !strings.Contains(res.Summary, "JWT") {
		t.Fatalf("Summary should contain JWT keyword, got %q", res.Summary)
	}
}

func TestExtract_Simple_ReasoningBlocks(t *testing.T) {
	res, err := Extract("testdata/simple.jsonl", "run-test-001", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	// simple.jsonl has assistant messages with text + tool_use → narrative blocks
	if len(res.Reasoning) == 0 {
		t.Fatal("expected at least 1 reasoning block")
	}
}

// ---------------------------------------------------------------------------
// Extract — thinking fixture
// ---------------------------------------------------------------------------

func TestExtract_WithThinking_HasThinkingBlocks(t *testing.T) {
	res, err := Extract("testdata/with_thinking.jsonl", "run-test-002", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	var thinkingCount int
	for _, rb := range res.Reasoning {
		if rb.Source == "thinking" {
			thinkingCount++
		}
	}
	if thinkingCount < 2 {
		t.Fatalf("expected ≥2 thinking blocks, got %d", thinkingCount)
	}
}

func TestExtract_WithThinking_FirstUserMsg(t *testing.T) {
	res, err := Extract("testdata/with_thinking.jsonl", "run-test-002", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if !strings.Contains(res.FirstUserMsg, "connection pooling") {
		t.Fatalf("unexpected FirstUserMsg: %q", res.FirstUserMsg)
	}
}

// ---------------------------------------------------------------------------
// Extract — malformed fixture
// ---------------------------------------------------------------------------

func TestExtract_Malformed_DoesNotError(t *testing.T) {
	res, err := Extract("testdata/malformed.jsonl", "run-test-003", "", "")
	if err != nil {
		t.Fatalf("Extract must not error on malformed lines, got: %v", err)
	}
	if res.FirstUserMsg == "" {
		t.Fatal("expected FirstUserMsg even with malformed lines")
	}
	if res.Summary == "" {
		t.Fatal("expected Summary even with malformed lines")
	}
}

// ---------------------------------------------------------------------------
// Extract — empty file
// ---------------------------------------------------------------------------

func TestExtract_EmptyFile_ReturnsZeroResult(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "empty-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err := Extract(f.Name(), "run-empty", "", "")
	if err != nil {
		t.Fatalf("Extract error on empty file: %v", err)
	}
	if res.FirstUserMsg != "" || res.Summary != "" || len(res.Reasoning) != 0 {
		t.Fatalf("expected zero Result for empty file, got %+v", res)
	}
}

// ---------------------------------------------------------------------------
// Extract — oversized content truncation
// ---------------------------------------------------------------------------

func TestExtract_OversizedContent_Truncated(t *testing.T) {
	// Build a JSONL file with a user message exceeding 2 KB.
	bigText := strings.Repeat("A", maxIntentBytes+500)
	line := `{"type":"user","message":{"content":[{"type":"text","text":"` + bigText + `"}]}}` + "\n"
	line += `{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}` + "\n"

	f, err := os.CreateTemp(t.TempDir(), "big-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err := Extract(f.Name(), "run-big", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(res.FirstUserMsg) > maxIntentBytes+len(truncationSuffix) {
		t.Fatalf("FirstUserMsg not truncated: len=%d", len(res.FirstUserMsg))
	}
	if !strings.HasSuffix(res.FirstUserMsg, truncationSuffix) {
		t.Fatalf("truncation suffix missing: %q", res.FirstUserMsg)
	}
}

// ---------------------------------------------------------------------------
// Extract — env gate DANDORI_INTENT_DISABLED
// ---------------------------------------------------------------------------

func TestExtract_EnvGate_Disabled(t *testing.T) {
	t.Setenv("DANDORI_INTENT_DISABLED", "1")

	res, err := Extract("testdata/simple.jsonl", "run-gated", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if res.FirstUserMsg != "" || res.Summary != "" || len(res.Reasoning) != 0 {
		t.Fatalf("expected empty Result when disabled, got %+v", res)
	}
}

func TestExtract_EnvGate_EnabledWhenEmpty(t *testing.T) {
	t.Setenv("DANDORI_INTENT_DISABLED", "")

	res, err := Extract("testdata/simple.jsonl", "run-enabled", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if res.FirstUserMsg == "" {
		t.Fatal("expected FirstUserMsg when env var is empty string")
	}
}

// ---------------------------------------------------------------------------
// Reasoning cap: never exceed maxReasoningBlocks
// ---------------------------------------------------------------------------

func TestExtract_ReasoningCap(t *testing.T) {
	// Build a file with 15 assistant messages each containing text+tool_use.
	var sb strings.Builder
	sb.WriteString(`{"type":"user","message":{"content":[{"type":"text","text":"do many things"}]}}` + "\n")
	for i := 0; i < 15; i++ {
		sb.WriteString(`{"type":"assistant","message":{"content":[{"type":"text","text":"step narrative"},{"type":"tool_use","id":"tu","name":"Bash","input":{}}]}}` + "\n")
		sb.WriteString(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu","content":"ok"}]}}` + "\n")
	}
	sb.WriteString(`{"type":"assistant","message":{"content":[{"type":"text","text":"all done"}]}}` + "\n")

	f, err := os.CreateTemp(t.TempDir(), "cap-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err := Extract(f.Name(), "run-cap", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(res.Reasoning) > maxReasoningBlocks {
		t.Fatalf("reasoning blocks exceed cap: got %d, max %d", len(res.Reasoning), maxReasoningBlocks)
	}
}
