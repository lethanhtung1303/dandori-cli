package intent

import "regexp"

const truncationSuffix = "…[truncated]"

// truncate caps s to n bytes (UTF-8 safe: cuts on rune boundary).
// If s fits within n bytes it is returned unchanged.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Walk back to a rune boundary.
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + truncationSuffix
}

// secretPatterns matches common secret patterns that must not be stored.
// Patterns: OpenAI/Anthropic sk- keys, GitHub PATs, Bearer tokens,
// AWS secret-access-key env, and generic key/token/password/secret assignments.
var secretPatterns = []*regexp.Regexp{
	// OpenAI / Anthropic style: sk-...
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{10,}`),
	// GitHub PATs: ghp_, gho_, ghs_, ghr_, github_pat_
	regexp.MustCompile(`\bgh[pos]_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	// Bearer tokens in headers / config
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9\-._~+/]+=*`),
	// AWS access/secret keys (26- and 40-char forms)
	regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`),
	// Generic key=value / key: value patterns for api_key, token, password, secret.
	// Requires assignment delimiter (=, :, or quote) — bare " password hashing" no longer matches.
	regexp.MustCompile(`(?i)\b(api[_-]?key|token|password|secret)\b\s*[:=]\s*["']?[^\s"',;}{]{4,}`),
	regexp.MustCompile(`(?i)["'](api[_-]?key|token|password|secret)["']\s*:\s*["'][^"']{4,}["']`),
}

// redactSecrets replaces recognised secret patterns with the literal string
// "<redacted>". The replacement is applied in pattern order so earlier (more
// specific) patterns win.
func redactSecrets(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "<redacted>")
	}
	return s
}
