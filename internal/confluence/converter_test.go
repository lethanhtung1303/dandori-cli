package confluence

import (
	"strings"
	"testing"
)

func TestStorageToMarkdownHeadings(t *testing.T) {
	tests := []struct {
		name     string
		storage  string
		contains string
	}{
		{"h1", "<h1>Title</h1>", "# Title"},
		{"h2", "<h2>Section</h2>", "## Section"},
		{"h3", "<h3>Subsection</h3>", "### Subsection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := StorageToMarkdown(tt.storage)
			if !strings.Contains(md, tt.contains) {
				t.Errorf("StorageToMarkdown(%q) = %q, want contains %q", tt.storage, md, tt.contains)
			}
		})
	}
}

func TestStorageToMarkdownParagraphs(t *testing.T) {
	storage := "<p>First paragraph.</p><p>Second paragraph.</p>"
	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "First paragraph.") {
		t.Error("should contain first paragraph")
	}
	if !strings.Contains(md, "Second paragraph.") {
		t.Error("should contain second paragraph")
	}
}

func TestStorageToMarkdownUnorderedList(t *testing.T) {
	storage := "<ul><li>Item 1</li><li>Item 2</li><li>Item 3</li></ul>"
	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "- Item 1") {
		t.Errorf("should contain '- Item 1', got: %s", md)
	}
	if !strings.Contains(md, "- Item 2") {
		t.Error("should contain '- Item 2'")
	}
}

func TestStorageToMarkdownOrderedList(t *testing.T) {
	storage := "<ol><li>First</li><li>Second</li></ol>"
	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "1. First") {
		t.Errorf("should contain '1. First', got: %s", md)
	}
}

func TestStorageToMarkdownCodeBlock(t *testing.T) {
	storage := `<ac:structured-macro ac:name="code">
		<ac:parameter ac:name="language">go</ac:parameter>
		<ac:plain-text-body><![CDATA[func main() {}]]></ac:plain-text-body>
	</ac:structured-macro>`

	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "```") {
		t.Error("should contain code fence")
	}
	if !strings.Contains(md, "func main()") {
		t.Error("should contain code content")
	}
}

func TestStorageToMarkdownLinks(t *testing.T) {
	storage := `<a href="https://example.com">Example Link</a>`
	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "[Example Link](https://example.com)") {
		t.Errorf("should contain markdown link, got: %s", md)
	}
}

func TestStorageToMarkdownTable(t *testing.T) {
	storage := `<table>
		<tr><th>Name</th><th>Value</th></tr>
		<tr><td>foo</td><td>bar</td></tr>
	</table>`

	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "| Name | Value |") {
		t.Errorf("should contain table header, got: %s", md)
	}
	if !strings.Contains(md, "| foo | bar |") {
		t.Error("should contain table row")
	}
}

func TestStorageToMarkdownBoldItalic(t *testing.T) {
	storage := "<p><strong>bold</strong> and <em>italic</em></p>"
	md := StorageToMarkdown(storage)

	if !strings.Contains(md, "**bold**") {
		t.Errorf("should contain **bold**, got: %s", md)
	}
	if !strings.Contains(md, "*italic*") {
		t.Error("should contain *italic*")
	}
}

func TestStorageToMarkdownEmpty(t *testing.T) {
	md := StorageToMarkdown("")
	if md != "" {
		t.Errorf("empty storage should return empty markdown, got: %s", md)
	}
}

func TestMarkdownToStorageHeadings(t *testing.T) {
	tests := []struct {
		md       string
		contains string
	}{
		{"# Title", "<h1>Title</h1>"},
		{"## Section", "<h2>Section</h2>"},
		{"### Sub", "<h3>Sub</h3>"},
	}

	for _, tt := range tests {
		t.Run(tt.md, func(t *testing.T) {
			storage := MarkdownToStorage(tt.md)
			if !strings.Contains(storage, tt.contains) {
				t.Errorf("MarkdownToStorage(%q) = %q, want contains %q", tt.md, storage, tt.contains)
			}
		})
	}
}

func TestMarkdownToStorageParagraph(t *testing.T) {
	md := "This is a paragraph."
	storage := MarkdownToStorage(md)

	if !strings.Contains(storage, "<p>This is a paragraph.</p>") {
		t.Errorf("should wrap in <p>, got: %s", storage)
	}
}

func TestMarkdownToStorageList(t *testing.T) {
	md := "- Item 1\n- Item 2"
	storage := MarkdownToStorage(md)

	if !strings.Contains(storage, "<ul>") {
		t.Errorf("should contain <ul>, got: %s", storage)
	}
	if !strings.Contains(storage, "<li>Item 1</li>") {
		t.Error("should contain list item")
	}
}

func TestMarkdownToStorageCodeBlock(t *testing.T) {
	md := "```go\nfunc main() {}\n```"
	storage := MarkdownToStorage(md)

	if !strings.Contains(storage, "ac:structured-macro") {
		t.Errorf("should contain code macro, got: %s", storage)
	}
	if !strings.Contains(storage, "func main()") {
		t.Error("should contain code content")
	}
}

func TestMarkdownToStorageBoldItalic(t *testing.T) {
	md := "**bold** and *italic*"
	storage := MarkdownToStorage(md)

	if !strings.Contains(storage, "<strong>bold</strong>") {
		t.Errorf("should contain <strong>, got: %s", storage)
	}
	if !strings.Contains(storage, "<em>italic</em>") {
		t.Error("should contain <em>")
	}
}

func TestMarkdownToStorageEmpty(t *testing.T) {
	storage := MarkdownToStorage("")
	if storage != "" {
		t.Errorf("empty markdown should return empty storage, got: %s", storage)
	}
}

func TestRoundTrip(t *testing.T) {
	original := "<h1>Title</h1><p>Some <strong>bold</strong> text.</p><ul><li>Item</li></ul>"

	md := StorageToMarkdown(original)
	if md == "" {
		t.Fatal("conversion to markdown failed")
	}

	storage := MarkdownToStorage(md)
	if storage == "" {
		t.Fatal("conversion to storage failed")
	}

	// Should contain the key content
	if !strings.Contains(storage, "Title") {
		t.Error("round trip should preserve title")
	}
	if !strings.Contains(storage, "bold") {
		t.Error("round trip should preserve bold text")
	}
}
