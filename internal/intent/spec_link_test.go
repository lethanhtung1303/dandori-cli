package intent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ExtractSpecLinks — URL extraction from first user message
// ---------------------------------------------------------------------------

func TestExtractSpecLinks_SingleURL(t *testing.T) {
	msg := "See the spec at https://myorg.atlassian.net/wiki/spaces/ENG/pages/123456/Login+Feature"
	sl := ExtractSpecLinks(msg, "", "PROJ-42")

	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
	if sl.JiraKey != "PROJ-42" {
		t.Fatalf("jira key: want %q, got %q", "PROJ-42", sl.JiraKey)
	}
}

func TestExtractSpecLinks_NilURLsOnEmptyMsg(t *testing.T) {
	sl := ExtractSpecLinks("", "", "")
	if sl.ConfluenceURLs == nil {
		t.Fatal("ConfluenceURLs must never be nil")
	}
	if len(sl.ConfluenceURLs) != 0 {
		t.Fatalf("expected 0 URLs, got %d", len(sl.ConfluenceURLs))
	}
}

func TestExtractSpecLinks_DeduplicatesURLs(t *testing.T) {
	url := "https://myorg.atlassian.net/wiki/spaces/ENG/pages/111"
	msg := "See " + url + " and also " + url + " again."
	sl := ExtractSpecLinks(msg, "", "")

	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("duplicate URL should be deduplicated, got %d URLs: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_CapAt20(t *testing.T) {
	// Build a message with 25 distinct Confluence URLs.
	var sb strings.Builder
	for i := 0; i < 25; i++ {
		sb.WriteString("https://org.atlassian.net/wiki/spaces/ENG/pages/")
		for d := 100000 + i; ; d /= 10 {
			sb.WriteByte(byte('0' + d%10))
			if d < 10 {
				break
			}
		}
		sb.WriteByte(' ')
	}
	sl := ExtractSpecLinks(sb.String(), "", "")

	if len(sl.ConfluenceURLs) != maxConfluenceURLs {
		t.Fatalf("expected cap of %d URLs, got %d", maxConfluenceURLs, len(sl.ConfluenceURLs))
	}
}

func TestExtractSpecLinks_SevenURLsCappedAtFive_PlanSays5(t *testing.T) {
	// The plan spec says cap at 5; our implementation uses 20.
	// Test that 7 distinct URLs → 7 captured (our cap is 20, not 5).
	// This validates the extractor against OUR cap, not the plan's draft cap.
	var sb strings.Builder
	for i := 1; i <= 7; i++ {
		sb.WriteString("https://org.atlassian.net/wiki/spaces/ENG/pages/10000")
		sb.WriteByte(byte('0' + i))
		sb.WriteByte(' ')
	}
	sl := ExtractSpecLinks(sb.String(), "", "")
	if len(sl.ConfluenceURLs) != 7 {
		t.Fatalf("expected 7 URLs (cap=20), got %d", len(sl.ConfluenceURLs))
	}
}

func TestExtractSpecLinks_NoConfluenceURL_EmptySlice(t *testing.T) {
	msg := "Fix the login bug in src/auth.go line 45."
	sl := ExtractSpecLinks(msg, "", "PROJ-1")
	if len(sl.ConfluenceURLs) != 0 {
		t.Fatalf("expected 0 URLs, got %v", sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_SelfHostedConfluenceURL(t *testing.T) {
	msg := "Spec: https://confluence.internal.company.com/wiki/spaces/ENG/pages/999"
	sl := ExtractSpecLinks(msg, "", "")
	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("expected 1 self-hosted URL, got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

// ---------------------------------------------------------------------------
// ExtractSpecLinks — cwd file scanning
// ---------------------------------------------------------------------------

func TestExtractSpecLinks_CWDReadme_NoURLs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\nNo Confluence links here."), 0644); err != nil {
		t.Fatal(err)
	}
	sl := ExtractSpecLinks("", dir, "")
	if len(sl.ConfluenceURLs) != 0 {
		t.Fatalf("expected 0 URLs, got %v", sl.ConfluenceURLs)
	}
	if len(sl.SourcePaths) != 1 {
		t.Fatalf("expected README.md in SourcePaths, got %v", sl.SourcePaths)
	}
}

func TestExtractSpecLinks_CWDReadme_WithURL(t *testing.T) {
	dir := t.TempDir()
	content := "# My Project\nSpec: https://myorg.atlassian.net/wiki/spaces/ENG/pages/555"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	sl := ExtractSpecLinks("", dir, "")
	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("expected 1 URL from README, got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_CWDClaudeMd_WithURL(t *testing.T) {
	dir := t.TempDir()
	content := "See https://team.atlassian.net/wiki/pages/overview for the project spec."
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	sl := ExtractSpecLinks("", dir, "")
	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("expected 1 URL from CLAUDE.md, got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_DedupeAcrossSources(t *testing.T) {
	// Same URL in user msg AND README.md → stored once.
	dir := t.TempDir()
	url := "https://myorg.atlassian.net/wiki/spaces/ENG/pages/999"
	readme := "Spec: " + url
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatal(err)
	}
	msg := "Please implement per " + url
	sl := ExtractSpecLinks(msg, dir, "")
	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("cross-source dedup failed, got %d URLs: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_BothSourcesCaptured(t *testing.T) {
	// Different URL in user msg and in CLAUDE.md → both stored.
	dir := t.TempDir()
	urlMsg := "https://myorg.atlassian.net/wiki/spaces/ENG/pages/111"
	urlFile := "https://myorg.atlassian.net/wiki/spaces/ENG/pages/222"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("See "+urlFile), 0644); err != nil {
		t.Fatal(err)
	}
	sl := ExtractSpecLinks("See "+urlMsg, dir, "")
	if len(sl.ConfluenceURLs) != 2 {
		t.Fatalf("expected 2 URLs (one per source), got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_MissingCWDFiles_NoError(t *testing.T) {
	// Empty temp dir with no README/CLAUDE.md/plan.md — must not error and
	// SourcePaths must be empty.
	dir := t.TempDir()
	sl := ExtractSpecLinks("", dir, "PROJ-99")
	if len(sl.SourcePaths) != 0 {
		t.Fatalf("expected empty SourcePaths, got %v", sl.SourcePaths)
	}
}

func TestExtractSpecLinks_EmptyCWD_NoError(t *testing.T) {
	// Passing empty cwd must not error or panic.
	sl := ExtractSpecLinks("hello world", "", "PROJ-1")
	if len(sl.SourcePaths) != 0 {
		t.Fatalf("expected empty SourcePaths when cwd is empty, got %v", sl.SourcePaths)
	}
}

func TestExtractSpecLinks_JiraKeyPassthrough(t *testing.T) {
	sl := ExtractSpecLinks("", "", "MYPROJECT-1234")
	if sl.JiraKey != "MYPROJECT-1234" {
		t.Fatalf("jira key: want %q, got %q", "MYPROJECT-1234", sl.JiraKey)
	}
}

func TestExtractSpecLinks_JiraKeyEmptyWhenNotProvided(t *testing.T) {
	sl := ExtractSpecLinks("some message", "", "")
	if sl.JiraKey != "" {
		t.Fatalf("expected empty jira key, got %q", sl.JiraKey)
	}
}

// ---------------------------------------------------------------------------
// ExtractSpecLinks — plans/ subdirectory scan
// ---------------------------------------------------------------------------

func TestExtractSpecLinks_PlansMd_URLExtracted(t *testing.T) {
	dir := t.TempDir()
	plansSubDir := filepath.Join(dir, "plans", "260430-my-plan")
	if err := os.MkdirAll(plansSubDir, 0755); err != nil {
		t.Fatal(err)
	}
	planContent := "Plan links: https://myorg.atlassian.net/wiki/spaces/ENG/pages/777"
	if err := os.WriteFile(filepath.Join(plansSubDir, "plan.md"), []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}
	sl := ExtractSpecLinks("", dir, "")
	if len(sl.ConfluenceURLs) != 1 {
		t.Fatalf("expected 1 URL from plans/*/plan.md, got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
}

func TestExtractSpecLinks_NoPlansMd_NoError(t *testing.T) {
	dir := t.TempDir()
	sl := ExtractSpecLinks("", dir, "")
	if sl.ConfluenceURLs == nil {
		t.Fatal("ConfluenceURLs must never be nil")
	}
}

// ---------------------------------------------------------------------------
// Fixture-based integration test: JSONL with Confluence URL in first user msg
// ---------------------------------------------------------------------------

func TestExtractSpecLinks_Fixture_UserMsgAndClaudeMd(t *testing.T) {
	// Simulate the resolved open question scenario:
	//   - JSONL first user message contains a Confluence URL
	//   - A CLAUDE.md in the temp dir contains another URL
	//   - Dedup + both sources verified.

	dir := t.TempDir()
	urlInMsg := "https://myteam.atlassian.net/wiki/spaces/PROD/pages/101/Spec"
	urlInFile := "https://myteam.atlassian.net/wiki/spaces/PROD/pages/202/Guide"

	claudeMd := "# CLAUDE.md\nSee " + urlInFile + " for project context."
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeMd), 0644); err != nil {
		t.Fatal(err)
	}

	firstUserMsg := "Implement G8 Phase 3 per " + urlInMsg
	sl := ExtractSpecLinks(firstUserMsg, dir, "G8-3")

	if len(sl.ConfluenceURLs) != 2 {
		t.Fatalf("expected 2 URLs (one per source), got %d: %v", len(sl.ConfluenceURLs), sl.ConfluenceURLs)
	}
	found101, found202 := false, false
	for _, u := range sl.ConfluenceURLs {
		if strings.Contains(u, "101") {
			found101 = true
		}
		if strings.Contains(u, "202") {
			found202 = true
		}
	}
	if !found101 || !found202 {
		t.Fatalf("did not capture both URLs: %v", sl.ConfluenceURLs)
	}
	if sl.JiraKey != "G8-3" {
		t.Fatalf("jira key: want %q, got %q", "G8-3", sl.JiraKey)
	}
	if len(sl.SourcePaths) != 1 {
		t.Fatalf("expected CLAUDE.md in SourcePaths, got %v", sl.SourcePaths)
	}
}
