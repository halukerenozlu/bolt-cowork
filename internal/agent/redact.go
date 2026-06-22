package agent

import (
	"sort"
	"strings"
	"sync"
)

// minSecretLen is the minimum length a secret must have to be registered.
// Short values (< 4 chars) are ignored to avoid false-positive redactions.
const minSecretLen = 4

// Redactor replaces known secret values with "[REDACTED]" in text output.
type Redactor struct {
	mu      sync.RWMutex
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
	sort.Slice(filtered, func(i, j int) bool {
		return len(filtered[i]) > len(filtered[j])
	})
	return &Redactor{secrets: filtered}
}

// Redact replaces all occurrences of registered secrets in s with "[REDACTED]".
func (r *Redactor) Redact(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, "[REDACTED]")
	}
	return s
}

// AddSecret registers a new secret for future redaction. Empty, short, and
// duplicate values are ignored.
func (r *Redactor) AddSecret(secret string) {
	if r == nil || len(secret) < minSecretLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.secrets {
		if existing == secret {
			return
		}
	}
	r.secrets = append(r.secrets, secret)
	sort.Slice(r.secrets, func(i, j int) bool {
		return len(r.secrets[i]) > len(r.secrets[j])
	})
}
