package mcp

import (
	"fmt"
	"strings"
)

// NormalizeConfig normalizes cfg in place. It performs three operations in
// order:
//
//  1. Trim — leading and trailing whitespace is stripped from each server's
//     Name, Transport, Command, and URL fields. Transport is also lowercased
//     so that "SSE" and "STDIO" are always stored as "sse" and "stdio".
//  2. Validate — an error is returned if:
//     - any server has an empty Name after trimming,
//     - any server has an unknown Transport (allowed values: "", "stdio", "sse"),
//     - a stdio (or unspecified) transport server has an empty Command, or
//     - an sse transport server has an empty URL.
//  3. Deduplicate — if two servers share the same Name, the first occurrence
//     is kept and later ones are silently discarded.
func NormalizeConfig(cfg *MCPConfig) error {
	seen := make(map[string]struct{}, len(cfg.Servers))
	deduped := make([]ServerConfig, 0, len(cfg.Servers))

	for i := range cfg.Servers {
		s := &cfg.Servers[i]

		s.Name = strings.TrimSpace(s.Name)
		s.Transport = strings.ToLower(strings.TrimSpace(s.Transport))
		s.Command = strings.TrimSpace(s.Command)
		s.URL = strings.TrimSpace(s.URL)

		if s.Name == "" {
			return fmt.Errorf("mcp: server at index %d has empty name", i)
		}

		switch s.Transport {
		case "", "stdio", "sse":
			// valid
		default:
			return fmt.Errorf("mcp: server %q: unknown transport %q", s.Name, s.Transport)
		}

		if s.Transport == "sse" {
			if s.URL == "" {
				return fmt.Errorf("mcp: server %q (sse) has empty url", s.Name)
			}
		} else {
			// stdio transport (or unspecified, which defaults to stdio behaviour).
			if s.Command == "" {
				return fmt.Errorf("mcp: server %q has empty command", s.Name)
			}
		}

		if _, dup := seen[s.Name]; dup {
			continue
		}
		seen[s.Name] = struct{}{}
		deduped = append(deduped, *s)
	}

	cfg.Servers = deduped
	return nil
}
