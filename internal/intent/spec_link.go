package intent

import (
	"os"
	"path/filepath"
	"regexp"
)

// maxConfluenceURLs is the hard cap on Confluence URLs stored per run.
// Sanity limit — agents rarely reference more than a handful of specs.
const maxConfluenceURLs = 20

// maxSourcePaths caps the number of cwd file paths recorded in SpecLinks.
const maxSourcePaths = 3

// confluenceURLRe matches Atlassian-hosted and self-hosted Confluence page URLs.
// Two patterns:
//  1. Atlassian Cloud:  https://<tenant>.atlassian.net/wiki/...
//  2. Self-hosted (older Data Center layout): https://<host>/wiki/...
//
// The character class [^\s<>"')\]] stops at whitespace and common delimiters
// used in Markdown / plain text so partial captures are avoided.
var confluenceURLRe = regexp.MustCompile(
	`https?://` +
		`(?:` +
		`[a-zA-Z0-9-]+\.atlassian\.net/wiki/` + // Atlassian Cloud
		`|` +
		`[a-zA-Z0-9][a-zA-Z0-9._-]*/wiki/` + // self-hosted
		`)` +
		`[^\s<>"')\]]+`,
)

// cwdSpecFiles is the ordered list of file names relative to cwd that are
// scanned for Confluence URLs. Scan order is deterministic; paths are
// included in SourcePaths only when the file exists.
var cwdSpecFiles = []string{
	"README.md",
	"CLAUDE.md",
	"plan.md",
}

// SpecLinks holds the spec-linkage snapshot for one run.
// Embedded into the intent.extracted event payload (no separate event type).
type SpecLinks struct {
	// JiraKey is the Jira issue key already stored on the run (back-pointer).
	// Passed in by the caller — P3 never re-detects it.
	JiraKey string `json:"jira_key,omitempty"`
	// ConfluenceURLs contains unique Confluence page URLs extracted from the
	// first user message and from any detected spec files in cwd.
	// Capped at maxConfluenceURLs (20). Never nil — empty slice when none found.
	ConfluenceURLs []string `json:"confluence_urls"`
	// SourcePaths lists the cwd-relative file paths that were scanned for URLs
	// (README.md, CLAUDE.md, plan.md). Capped at maxSourcePaths (3).
	// A path is included only when the file exists and was successfully read.
	SourcePaths []string `json:"source_paths"`
}

// ExtractSpecLinks builds a SpecLinks snapshot for the run.
//
//   - firstUserMsg: first human text from the session transcript (already
//     capped at 2 KB by the extractor; may be empty).
//   - cwd: working directory at the time the agent ran (may be "").
//   - jiraKey: issue key already stored on the run; passed through verbatim.
//
// The function is fail-soft: unreadable files and regex errors are silently
// skipped so the wrapper's run-completion path is never interrupted.
func ExtractSpecLinks(firstUserMsg, cwd, jiraKey string) SpecLinks {
	result := SpecLinks{
		JiraKey:        jiraKey,
		ConfluenceURLs: []string{},
		SourcePaths:    []string{},
	}

	seen := make(map[string]struct{})

	// --- Source 1: first user message ---
	appendURLs(&result.ConfluenceURLs, firstUserMsg, seen)

	// --- Source 2: well-known spec files in cwd ---
	if cwd != "" {
		scanCWDFiles(cwd, &result, seen)
	}

	// --- Also check plans/ subdirectory for plan.md ---
	if cwd != "" {
		scanPlansMd(cwd, &result, seen)
	}

	return result
}

// appendURLs extracts Confluence URLs from text and appends unique ones to dst.
// seen is updated in place. Cap (maxConfluenceURLs) is enforced on dst length.
func appendURLs(dst *[]string, text string, seen map[string]struct{}) {
	if text == "" || len(*dst) >= maxConfluenceURLs {
		return
	}
	matches := confluenceURLRe.FindAllString(text, -1)
	for _, u := range matches {
		if len(*dst) >= maxConfluenceURLs {
			break
		}
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			*dst = append(*dst, u)
		}
	}
}

// scanCWDFiles checks README.md, CLAUDE.md, plan.md in cwd, reads each for
// Confluence URLs, and records the path in SourcePaths when found.
func scanCWDFiles(cwd string, result *SpecLinks, seen map[string]struct{}) {
	for _, name := range cwdSpecFiles {
		if len(result.SourcePaths) >= maxSourcePaths {
			break
		}
		path := filepath.Join(cwd, name)
		content, err := os.ReadFile(path) // #nosec G304 — path is constructed from trusted cwd
		if err != nil {
			// File absent or unreadable — fail-soft, skip silently.
			continue
		}
		// Record the path (metadata only — no content stored).
		result.SourcePaths = append(result.SourcePaths, path)
		// Extract URLs from content.
		appendURLs(&result.ConfluenceURLs, string(content), seen)
	}
}

// scanPlansMd looks for the first plan.md file inside cwd/plans/*/
// and extracts Confluence URLs from it. The path is added to SourcePaths
// if it isn't already there from the top-level scan above.
func scanPlansMd(cwd string, result *SpecLinks, seen map[string]struct{}) {
	if len(result.SourcePaths) >= maxSourcePaths {
		return
	}
	plansDir := filepath.Join(cwd, "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		planPath := filepath.Join(plansDir, entry.Name(), "plan.md")
		content, err := os.ReadFile(planPath) // #nosec G304
		if err != nil {
			continue
		}
		// Add path only if not already present.
		if !containsPath(result.SourcePaths, planPath) {
			if len(result.SourcePaths) < maxSourcePaths {
				result.SourcePaths = append(result.SourcePaths, planPath)
			}
		}
		appendURLs(&result.ConfluenceURLs, string(content), seen)
		// Stop after first matching plan.md.
		break
	}
}

// containsPath reports whether paths contains target.
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
