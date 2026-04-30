package intent

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func blocks(texts ...string) []ReasoningBlock {
	out := make([]ReasoningBlock, len(texts))
	for i, t := range texts {
		out[i] = ReasoningBlock{Source: "thinking", Text: t}
	}
	return out
}

// ---------------------------------------------------------------------------
// Pattern 1 — "I'll go with X because Y"
// ---------------------------------------------------------------------------

func TestExtractDecisions_Pattern1_IllGoWith(t *testing.T) {
	bs := blocks("I'll go with bcrypt because it has a tunable cost factor.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	d := got[0]
	if !strings.Contains(strings.ToLower(d.Chosen), "bcrypt") {
		t.Errorf("Chosen should contain 'bcrypt', got %q", d.Chosen)
	}
	if !strings.Contains(strings.ToLower(d.Rationale), "cost factor") {
		t.Errorf("Rationale should contain 'cost factor', got %q", d.Rationale)
	}
	if len(d.Rejected) != 0 {
		t.Errorf("expected no Rejected for pattern 1, got %v", d.Rejected)
	}
}

func TestExtractDecisions_Pattern1_IllGoWithContraction(t *testing.T) {
	// Both "I'll" and "I'll" (straight/curly) should match.
	bs := blocks("I'll go with argon2 because it is memory-hard.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Pattern 2 — "using X over/instead of Y"
// ---------------------------------------------------------------------------

func TestExtractDecisions_Pattern2_UsingOver(t *testing.T) {
	bs := blocks("I am using JWT over opaque tokens because JWT allows stateless verification.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	d := got[0]
	if !strings.Contains(strings.ToLower(d.Chosen), "jwt") {
		t.Errorf("Chosen should contain 'jwt', got %q", d.Chosen)
	}
	if len(d.Rejected) == 0 {
		t.Errorf("expected Rejected to be non-empty")
	}
}

func TestExtractDecisions_Pattern2_UsingInsteadOf(t *testing.T) {
	bs := blocks("using postgres instead of sqlite for production deployments.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	if !strings.Contains(strings.ToLower(got[0].Chosen), "postgres") {
		t.Errorf("Chosen should contain 'postgres', got %q", got[0].Chosen)
	}
}

// ---------------------------------------------------------------------------
// Pattern 3 — "better to X than/rather than Y"
// ---------------------------------------------------------------------------

func TestExtractDecisions_Pattern3_BetterThan(t *testing.T) {
	bs := blocks("It is better to fail fast than silently swallow the error.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	if !strings.Contains(strings.ToLower(got[0].Chosen), "fail fast") {
		t.Errorf("Chosen should contain 'fail fast', got %q", got[0].Chosen)
	}
	if len(got[0].Rejected) == 0 {
		t.Error("expected Rejected to be non-empty")
	}
}

func TestExtractDecisions_Pattern3_RatherThan(t *testing.T) {
	bs := blocks("Better to validate at the boundary rather than deep inside the handler.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Pattern 4 — "decided to X"
// ---------------------------------------------------------------------------

func TestExtractDecisions_Pattern4_DecidedTo(t *testing.T) {
	bs := blocks("After weighing the options, decided to use a single SQLite file.")
	got := ExtractDecisions(bs)
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	if !strings.Contains(strings.ToLower(got[0].Chosen), "use a single sqlite file") {
		t.Errorf("Chosen should contain the decided action, got %q", got[0].Chosen)
	}
	if len(got[0].Rejected) != 0 {
		t.Error("pattern 4 should have no Rejected")
	}
}

// ---------------------------------------------------------------------------
// Pattern 5 — "could [either] X or Y" with follow-up
// ---------------------------------------------------------------------------

func TestExtractDecisions_Pattern5_WithFollowUp(t *testing.T) {
	bs := blocks(
		"I could either use Redis or in-memory caching.",
		"I'll go with Redis because we already have an instance running.",
	)
	got := ExtractDecisions(bs)
	// The follow-up in block 2 should trigger P1 match, giving 1 decision from
	// that block; pattern 5 in block 1 should also resolve via follow-up.
	// Total = at most 2. Must have at least 1 decision.
	if len(got) == 0 {
		t.Fatal("expected at least 1 decision (pattern 5 + follow-up), got 0")
	}
}

func TestExtractDecisions_Pattern5_NoFollowUp_Dropped(t *testing.T) {
	bs := blocks(
		"We could either go with approach A or approach B.",
		"The sky is blue and tests are running.",
		// two more neutral blocks that don't confirm anything
		"Checking the file structure now.",
	)
	got := ExtractDecisions(bs)
	// pattern 5 without follow-up → should be dropped; only 0 decisions expected
	// (the other 2 blocks have no patterns either).
	for _, d := range got {
		if strings.Contains(strings.ToLower(d.Chosen), "approach") {
			t.Errorf("tentative pattern 5 without follow-up should be dropped, got: %+v", d)
		}
	}
}

// ---------------------------------------------------------------------------
// Cap: 5 decisions per run
// ---------------------------------------------------------------------------

func TestExtractDecisions_Cap_FiveMax(t *testing.T) {
	// Build 8 blocks each containing a clear pattern-4 decision.
	texts := make([]string, 8)
	for i := range texts {
		texts[i] = "After evaluation, decided to use approach X."
	}
	got := ExtractDecisions(blocks(texts...))
	if len(got) != maxDecisionsPerRun {
		t.Fatalf("expected cap of %d decisions, got %d", maxDecisionsPerRun, len(got))
	}
}

// ---------------------------------------------------------------------------
// Empty and nil inputs
// ---------------------------------------------------------------------------

func TestExtractDecisions_EmptyBlocks_ReturnsNil(t *testing.T) {
	got := ExtractDecisions(nil)
	if got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	got = ExtractDecisions([]ReasoningBlock{})
	if got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestExtractDecisions_NoPattern_ReturnsEmpty(t *testing.T) {
	bs := blocks(
		"Checking the file system to understand the project layout.",
		"Running the test suite to verify correctness.",
	)
	got := ExtractDecisions(bs)
	if len(got) != 0 {
		t.Fatalf("expected 0 decisions for neutral text, got %d: %+v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Field truncation
// ---------------------------------------------------------------------------

func TestExtractDecisions_Truncation_ChosenField(t *testing.T) {
	long := strings.Repeat("x", maxDecisionFieldBytes+100)
	bs := blocks("I'll go with " + long + " because it is better.")
	got := ExtractDecisions(bs)
	if len(got) == 0 {
		t.Fatal("expected 1 decision")
	}
	if len(got[0].Chosen) > maxDecisionFieldBytes+len(truncationSuffix) {
		t.Errorf("Chosen not truncated: len=%d", len(got[0].Chosen))
	}
	if !strings.HasSuffix(got[0].Chosen, truncationSuffix) {
		t.Errorf("Chosen missing truncation suffix: %q", got[0].Chosen)
	}
}

// ---------------------------------------------------------------------------
// Redaction — secrets must not survive in Decision fields
// ---------------------------------------------------------------------------

func TestExtractDecisions_Redaction_SecretInChosen(t *testing.T) {
	bs := blocks("I'll go with sk-proj-abcdefghijklmnopqrstuvwxyz1234567890 because it authenticates.")
	got := ExtractDecisions(bs)
	if len(got) == 0 {
		t.Fatal("expected 1 decision")
	}
	if strings.Contains(got[0].Chosen, "sk-proj") {
		t.Errorf("secret not redacted in Chosen: %q", got[0].Chosen)
	}
}

// ---------------------------------------------------------------------------
// Fixture-based test: decisions-mixed.jsonl must yield exactly 2 decisions
// ---------------------------------------------------------------------------

func TestExtractDecisions_Fixture_MixedYieldsTwo(t *testing.T) {
	res, err := Extract("testdata/decisions-mixed.jsonl", "run-fixture-01", "", "")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	decisions := ExtractDecisions(res.Reasoning)
	if len(decisions) != 2 {
		t.Fatalf("expected exactly 2 decisions from fixture, got %d: %+v", len(decisions), decisions)
	}
}
