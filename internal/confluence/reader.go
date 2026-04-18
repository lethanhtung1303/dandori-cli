package confluence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Reader struct {
	client   ConfluenceClient
	cacheDir string
	ttl      time.Duration
}

type ReaderConfig struct {
	Client   ConfluenceClient
	CacheDir string
	TTL      time.Duration
}

func NewReader(cfg ReaderConfig) *Reader {
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = time.Hour
	}

	return &Reader{
		client:   cfg.Client,
		cacheDir: cfg.CacheDir,
		ttl:      ttl,
	}
}

func (r *Reader) FetchAndCache(ctx context.Context, pageID string) (string, error) {
	cachePath := r.cachePath(pageID)

	// Check if cache is fresh
	if r.isCacheFresh(cachePath) {
		return cachePath, nil
	}

	// Fetch from API
	page, err := r.client.GetPage(ctx, pageID)
	if err != nil {
		return "", fmt.Errorf("fetch page %s: %w", pageID, err)
	}

	// Convert to markdown
	md := StorageToMarkdown(page.Body.Storage.Value)

	// Add metadata header
	content := fmt.Sprintf("# %s\n\n%s", page.Title, md)

	// Ensure cache directory exists
	if err := os.MkdirAll(r.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	// Write to cache
	if err := os.WriteFile(cachePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write cache: %w", err)
	}

	return cachePath, nil
}

func (r *Reader) GetCachedMarkdown(pageID string) (string, error) {
	cachePath := r.cachePath(pageID)

	content, err := os.ReadFile(cachePath)
	if err != nil {
		return "", fmt.Errorf("read cache %s: %w", pageID, err)
	}

	return string(content), nil
}

func (r *Reader) IsCached(pageID string) bool {
	cachePath := r.cachePath(pageID)
	_, err := os.Stat(cachePath)
	return err == nil
}

func (r *Reader) InvalidateCache(pageID string) error {
	cachePath := r.cachePath(pageID)
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (r *Reader) cachePath(pageID string) string {
	return filepath.Join(r.cacheDir, pageID+".md")
}

func (r *Reader) isCacheFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	age := time.Since(info.ModTime())
	return age < r.ttl
}

type ContextAssembler struct {
	reader     *Reader
	contextDir string
}

type ContextAssemblerConfig struct {
	Reader     *Reader
	ContextDir string
}

func NewContextAssembler(cfg ContextAssemblerConfig) *ContextAssembler {
	return &ContextAssembler{
		reader:     cfg.Reader,
		contextDir: cfg.ContextDir,
	}
}

func (a *ContextAssembler) AssembleContext(ctx context.Context, issueKey string, pageIDs []string, summary string) (string, error) {
	if len(pageIDs) == 0 {
		return "", nil
	}

	// Ensure context directory exists
	if err := os.MkdirAll(a.contextDir, 0755); err != nil {
		return "", fmt.Errorf("create context dir: %w", err)
	}

	contextPath := filepath.Join(a.contextDir, issueKey+".md")

	var content string
	content += fmt.Sprintf("# Task Context: %s\n\n", issueKey)
	if summary != "" {
		content += fmt.Sprintf("**Summary:** %s\n\n", summary)
	}
	content += "---\n\n"

	for _, pageID := range pageIDs {
		// Fetch and cache each page
		_, err := a.reader.FetchAndCache(ctx, pageID)
		if err != nil {
			content += fmt.Sprintf("## Page %s\n\n*Error fetching page: %v*\n\n---\n\n", pageID, err)
			continue
		}

		// Read cached content
		pageContent, err := a.reader.GetCachedMarkdown(pageID)
		if err != nil {
			content += fmt.Sprintf("## Page %s\n\n*Error reading cache: %v*\n\n---\n\n", pageID, err)
			continue
		}

		content += pageContent + "\n\n---\n\n"
	}

	// Write assembled context
	if err := os.WriteFile(contextPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write context: %w", err)
	}

	return contextPath, nil
}
