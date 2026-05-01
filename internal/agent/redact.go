package agent

import "strings"

// minSecretLen is the minimum length a secret must have to be registered.
// Short values (< 4 chars) are ignored to avoid false-positive redactions.
const minSecretLen = 4

// Redactor replaces known secret values with "[REDACTED]" in text output.
type Redactor struct {
	secrets []string
}

// NewRedactor creates a Redactor from the given secret values.
// Empty strings and strings shorter than minSecretLen are silently ignored.
// Duplicate values are deduplicated.
func NewRedactor(secrets []string) *Redactor {
	seen := make(map[string]struct{})
	var filtered []string
	for _, s := range secrets {
		if len(s) < minSecretLen {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		filtered = append(filtered, s)
	}
	return &Redactor{secrets: filtered}
}

// Redact replaces all occurrences of registered secrets in s with "[REDACTED]".
func (r *Redactor) Redact(s string) string {
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, "[REDACTED]")
	}
	return s
}
