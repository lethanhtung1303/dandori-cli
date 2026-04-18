//go:build integration

package confluence

import (
	"context"
	"os"
	"testing"
	"time"
)

func getIntegrationClient(t *testing.T) *Client {
	baseURL := os.Getenv("DANDORI_CONFLUENCE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("DANDORI_JIRA_URL")
		if baseURL != "" {
			baseURL += "/wiki"
		}
	}
	user := os.Getenv("DANDORI_JIRA_USER")
	token := os.Getenv("DANDORI_JIRA_TOKEN")

	if baseURL == "" || user == "" || token == "" {
		t.Skip("DANDORI_CONFLUENCE_URL/DANDORI_JIRA_URL, DANDORI_JIRA_USER, DANDORI_JIRA_TOKEN required")
	}

	return NewClient(ClientConfig{
		BaseURL: baseURL,
		User:    user,
		Token:   token,
		IsCloud: true,
	})
}

func TestIntegration_SearchPages(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	pages, err := client.SearchPages(ctx, "CLITEST", "")
	if err != nil {
		t.Fatalf("SearchPages: %v", err)
	}

	t.Logf("Found %d pages in CLITEST space", len(pages))
	for _, p := range pages {
		t.Logf("  %s - %s", p.ID, p.Title)
	}
}

func TestIntegration_CreateAndGetPage(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create a test page
	testTitle := "Integration Test Page - " + time.Now().Format("20060102-150405")
	page, err := client.CreatePage(ctx, CreatePageRequest{
		SpaceKey: "CLITEST",
		Title:    testTitle,
		Body:     "<h1>Test Page</h1><p>Created by dandori-cli integration test.</p>",
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	t.Logf("Created page: %s (ID: %s)", page.Title, page.ID)

	// Get the page back
	fetched, err := client.GetPage(ctx, page.ID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}

	if fetched.Title != testTitle {
		t.Errorf("title = %s, want %s", fetched.Title, testTitle)
	}
	t.Logf("Fetched page body length: %d chars", len(fetched.Body.Storage.Value))
}

func TestIntegration_CreateReport(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	writer := NewWriter(WriterConfig{
		Client:   client,
		SpaceKey: "CLITEST",
	})

	// Use timestamp in run ID to ensure unique page title
	runID := "integ-" + time.Now().Format("20060102-150405")
	report := RunReport{
		RunID:        runID,
		IssueKey:     "CLITEST-1",
		AgentName:    "integration-test",
		Status:       "done",
		Duration:     90 * time.Second,
		CostUSD:      1.25,
		InputTokens:  5000,
		OutputTokens: 2000,
		Model:        "claude-sonnet-4-5-20250514",
		FilesChanged: []string{"main.go", "config.go"},
		GitDiff:      "+func hello() {}\n-func old() {}",
		StartedAt:    time.Now().Add(-90 * time.Second),
		EndedAt:      time.Now(),
	}

	page, err := writer.CreateReport(ctx, report)
	if err != nil {
		t.Fatalf("CreateReport: %v", err)
	}

	t.Logf("Created report page: %s (ID: %s)", page.Title, page.ID)
}

func TestIntegration_ReaderCache(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	reader := NewReader(ReaderConfig{
		Client:   client,
		CacheDir: cacheDir,
		TTL:      time.Hour,
	})

	// First, find a page in the CLITEST space
	pages, err := client.SearchPages(ctx, "CLITEST", "")
	if err != nil {
		t.Fatalf("SearchPages: %v", err)
	}
	if len(pages) == 0 {
		t.Skip("No pages in CLITEST space")
	}

	pageID := pages[0].ID
	t.Logf("Testing cache with page ID: %s", pageID)

	// Fetch and cache
	cachePath, err := reader.FetchAndCache(ctx, pageID)
	if err != nil {
		t.Fatalf("FetchAndCache: %v", err)
	}
	t.Logf("Cached to: %s", cachePath)

	// Check cache hit
	if !reader.IsCached(pageID) {
		t.Error("page should be cached")
	}

	// Read cached content
	content, err := reader.GetCachedMarkdown(pageID)
	if err != nil {
		t.Fatalf("GetCachedMarkdown: %v", err)
	}
	t.Logf("Cached content length: %d chars", len(content))
}
