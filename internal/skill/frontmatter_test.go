package skill

import (
	"strings"
	"testing"
)

func TestParseFrontMatter_Full(t *testing.T) {
	raw := []byte(`---
name: my-skill
description: Does something useful
tags:
  - files
  - automation
priority: 10
auto_trigger: true
requires_approval: true
---

# My Skill

Body content here.
`)
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-skill")
	}
	if meta.Description != "Does something useful" {
		t.Errorf("Description = %q", meta.Description)
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "files" || meta.Tags[1] != "automation" {
		t.Errorf("Tags = %v, want [files automation]", meta.Tags)
	}
	if meta.Priority != 10 {
		t.Errorf("Priority = %d, want 10", meta.Priority)
	}
	if !meta.AutoTrigger {
		t.Error("AutoTrigger = false, want true")
	}
	if !meta.RequiresApproval {
		t.Error("RequiresApproval = false, want true")
	}
	if !strings.Contains(body, "Body content here.") {
		t.Errorf("body missing expected content: %q", body)
	}
}

func TestParseFrontMatter_Minimal(t *testing.T) {
	raw := []byte("---\nname: minimal\n---\n\nBody.\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "minimal" {
		t.Errorf("Name = %q, want %q", meta.Name, "minimal")
	}
	// Description falls back to body first paragraph.
	if meta.Description != "Body." {
		t.Errorf("Description = %q, want %q (fallback from body)", meta.Description, "Body.")
	}
	if !strings.Contains(body, "Body.") {
		t.Errorf("body = %q, expected to contain 'Body.'", body)
	}
}

func TestParseFrontMatter_NoFrontMatter(t *testing.T) {
	raw := []byte("# Just a heading\n\nSome paragraph text here.\n")
	meta, body, _ := parseFrontMatter(raw, "my-skill/SKILL.md")

	// Name derived from path.
	if meta.Name != "my-skill" {
		t.Errorf("Name = %q, want %q (from path)", meta.Name, "my-skill")
	}
	// Description derived from first paragraph.
	if meta.Description != "Some paragraph text here." {
		t.Errorf("Description = %q, want %q", meta.Description, "Some paragraph text here.")
	}
	// Body is entire content.
	if body != string(raw) {
		t.Errorf("body should be entire content")
	}
}

func TestParseFrontMatter_InvalidYAML(t *testing.T) {
	raw := []byte("---\n: invalid: yaml: [\n---\n\nBody text.\n")
	meta, body, warns := parseFrontMatter(raw, "fallback-skill/SKILL.md")

	// Should have a warning about invalid YAML.
	hasYAMLWarn := false
	for _, w := range warns {
		if strings.Contains(w, "invalid YAML") {
			hasYAMLWarn = true
			break
		}
	}
	if !hasYAMLWarn {
		t.Errorf("expected YAML warning, got: %v", warns)
	}
	// Name derived from path.
	if meta.Name != "fallback-skill" {
		t.Errorf("Name = %q, want %q (fallback)", meta.Name, "fallback-skill")
	}
	// Body should still be available.
	if !strings.Contains(body, "Body text.") {
		t.Errorf("body = %q, expected to contain 'Body text.'", body)
	}
}

func TestParseFrontMatter_EmptyDescription(t *testing.T) {
	raw := []byte("---\nname: test\n---\n\nFirst paragraph of body content that should become description.\n\nSecond paragraph.\n")
	meta, _, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Description != "First paragraph of body content that should become description." {
		t.Errorf("Description = %q", meta.Description)
	}
}

func TestParseFrontMatter_DescriptionTruncation(t *testing.T) {
	// Create a body with a very long first paragraph.
	longPara := strings.Repeat("word ", 200) // ~1000 chars
	raw := []byte("---\nname: test\n---\n\n" + longPara + "\n")
	meta, _, _ := parseFrontMatter(raw, "test/SKILL.md")

	if len(meta.Description) > 512 {
		t.Errorf("Description length = %d, should be <= 512", len(meta.Description))
	}
}

func TestNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file-organizer/SKILL.md", "file-organizer"},
		{"skills/summarizer/SKILL.md", "summarizer"},
		{"my-custom-skill.md", "my-custom-skill"},
		{"SKILL.md", ""}, // parent is "." → empty
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := nameFromPath(tt.path)
			if got != tt.want {
				t.Errorf("nameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseFrontMatter_CRLF(t *testing.T) {
	raw := []byte("---\r\nname: crlf-skill\r\ndescription: Handles CRLF line endings\r\nauto_trigger: true\r\ntags:\r\n  - windows\r\npriority: 5\r\nrequires_approval: true\r\n---\r\n\r\n# CRLF Skill\r\n\r\nBody with CRLF endings.\r\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "crlf-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "crlf-skill")
	}
	if meta.Description != "Handles CRLF line endings" {
		t.Errorf("Description = %q, want %q", meta.Description, "Handles CRLF line endings")
	}
	if !meta.AutoTrigger {
		t.Error("AutoTrigger = false, want true")
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "windows" {
		t.Errorf("Tags = %v, want [windows]", meta.Tags)
	}
	if meta.Priority != 5 {
		t.Errorf("Priority = %d, want 5", meta.Priority)
	}
	if !meta.RequiresApproval {
		t.Error("RequiresApproval = false, want true")
	}
	if !strings.Contains(body, "Body with CRLF endings.") {
		t.Errorf("body = %q, expected to contain 'Body with CRLF endings.'", body)
	}
}

func TestSkillScope_String(t *testing.T) {
	tests := []struct {
		scope SkillScope
		want  string
	}{
		{ScopeBundled, "bundled"},
		{ScopeGlobal, "global"},
		{ScopeProject, "project"},
		{SkillScope(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.scope.String(); got != tt.want {
				t.Errorf("SkillScope(%d).String() = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}
