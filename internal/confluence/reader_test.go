package confluence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReaderFetchAndCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	mockClient := &mockConfluenceClient{
		pages: map[string]*Page{
			"123": {
				ID:    "123",
				Title: "Test Page",
				Body: PageBody{
					Storage: StorageBody{Value: "<h1>Hello</h1><p>World</p>"},
				},
			},
		},
	}

	reader := NewReader(ReaderConfig{
		Client:   mockClient,
		CacheDir: cacheDir,
		TTL:      time.Hour,
	})

	// First fetch - should call API
	path, err := reader.FetchAndCache(context.Background(), "123")
	if err != nil {
		t.Fatalf("FetchAndCache failed: %v", err)
	}
	if path == "" {
		t.Error("path should not be empty")
	}

	// Verify cache file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("cache file should exist")
	}

	// Read cached content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache failed: %v", err)
	}
	if len(content) == 0 {
		t.Error("cached content should not be empty")
	}
}

func TestReaderCacheHit(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	// Pre-populate cache
	cachePath := filepath.Join(cacheDir, "123.md")
	os.WriteFile(cachePath, []byte("# Cached Content"), 0644)

	callCount := 0
	mockClient := &mockConfluenceClient{
		onGetPage: func(id string) {
			callCount++
		},
		pages: map[string]*Page{
			"123": {ID: "123", Title: "Fresh"},
		},
	}

	reader := NewReader(ReaderConfig{
		Client:   mockClient,
		CacheDir: cacheDir,
		TTL:      time.Hour,
	})

	// Should use cache, not call API
	_, err := reader.FetchAndCache(context.Background(), "123")
	if err != nil {
		t.Fatalf("FetchAndCache failed: %v", err)
	}

	if callCount > 0 {
		t.Error("should use cache, not call API")
	}
}

func TestReaderCacheExpired(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	// Pre-populate expired cache
	cachePath := filepath.Join(cacheDir, "123.md")
	os.WriteFile(cachePath, []byte("# Old Content"), 0644)
	// Set mtime to past
	oldTime := time.Now().Add(-2 * time.Hour)
	os.Chtimes(cachePath, oldTime, oldTime)

	callCount := 0
	mockClient := &mockConfluenceClient{
		onGetPage: func(id string) {
			callCount++
		},
		pages: map[string]*Page{
			"123": {
				ID:    "123",
				Title: "Fresh",
				Body:  PageBody{Storage: StorageBody{Value: "<p>New</p>"}},
			},
		},
	}

	reader := NewReader(ReaderConfig{
		Client:   mockClient,
		CacheDir: cacheDir,
		TTL:      time.Hour, // 1 hour TTL, cache is 2 hours old
	})

	_, err := reader.FetchAndCache(context.Background(), "123")
	if err != nil {
		t.Fatalf("FetchAndCache failed: %v", err)
	}

	if callCount == 0 {
		t.Error("should refresh expired cache")
	}
}

func TestReaderGetCachedMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	cachePath := filepath.Join(cacheDir, "456.md")
	expectedContent := "# My Page\n\nSome content here."
	os.WriteFile(cachePath, []byte(expectedContent), 0644)

	reader := NewReader(ReaderConfig{CacheDir: cacheDir})

	content, err := reader.GetCachedMarkdown("456")
	if err != nil {
		t.Fatalf("GetCachedMarkdown failed: %v", err)
	}
	if content != expectedContent {
		t.Errorf("content = %q, want %q", content, expectedContent)
	}
}

func TestReaderGetCachedMarkdownNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	reader := NewReader(ReaderConfig{CacheDir: tmpDir})

	_, err := reader.GetCachedMarkdown("nonexistent")
	if err == nil {
		t.Error("should error for nonexistent cache")
	}
}

// Mock client for testing
type mockConfluenceClient struct {
	pages     map[string]*Page
	onGetPage func(id string)
}

func (m *mockConfluenceClient) GetPage(ctx context.Context, pageID string) (*Page, error) {
	if m.onGetPage != nil {
		m.onGetPage(pageID)
	}
	if page, ok := m.pages[pageID]; ok {
		return page, nil
	}
	return nil, ErrPageNotFound
}

func (m *mockConfluenceClient) CreatePage(ctx context.Context, req CreatePageRequest) (*Page, error) {
	return &Page{ID: "new", Title: req.Title}, nil
}

func (m *mockConfluenceClient) UpdatePage(ctx context.Context, pageID string, req UpdatePageRequest) (*Page, error) {
	return &Page{ID: pageID, Title: req.Title}, nil
}

func (m *mockConfluenceClient) SearchPages(ctx context.Context, spaceKey, title string) ([]Page, error) {
	return nil, nil
}
