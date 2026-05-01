package skill

import (
	"strings"
	"testing"
)

func TestSkillParser_UnicodeContent(t *testing.T) {
	raw := []byte("---\nname: 日本語-skill\ndescription: Unicode skill description\n---\n\n# Unicode Skill 🎉\n\nBody with emoji 🎉 and unicode.\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "日本語-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "日本語-skill")
	}
	if !strings.Contains(body, "🎉") {
		t.Errorf("body should contain emoji, got: %q", body)
	}
}

func TestSkillParser_LargeBody(t *testing.T) {
	// Create a 15KB body.
	largeParagraph := strings.Repeat("word ", 3000) // ~15KB
	raw := []byte("---\nname: large-body\ndescription: Has a large body\n---\n\n" + largeParagraph + "\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "large-body" {
		t.Errorf("Name = %q, want %q", meta.Name, "large-body")
	}
	// Full body should be preserved (no truncation of body content).
	if len(body) < 15000 {
		t.Errorf("body length = %d, expected >= 15000 (no truncation)", len(body))
	}

	// Now test description fallback with large body and no explicit description.
	raw2 := []byte("---\nname: large-fallback\n---\n\n" + largeParagraph + "\n")
	meta2, _, _ := parseFrontMatter(raw2, "test/SKILL.md")
	if len(meta2.Description) > 512 {
		t.Errorf("Description length = %d, should be <= 512 (fallback truncation)", len(meta2.Description))
	}
}

func TestSkillParser_MultipleDelimiters(t *testing.T) {
	raw := []byte("---\nname: multi-delim\ndescription: Test multiple delimiters\n---\n\n# Content\n\n---\n\nThis is after a horizontal rule.\n\n---\n\nAnother section.\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "multi-delim" {
		t.Errorf("Name = %q, want %q", meta.Name, "multi-delim")
	}
	// Body should contain the --- lines as content.
	if !strings.Contains(body, "---") {
		t.Error("body should preserve --- as content")
	}
	if !strings.Contains(body, "Another section.") {
		t.Error("body should contain content after second ---")
	}
}

func TestSkillParser_WhitespaceBeforeFrontMatter(t *testing.T) {
	// Leading whitespace means strings.HasPrefix(content, "---\n") fails,
	// so fallback to no-frontmatter path.
	raw := []byte("\n\n---\nname: test\n---\n\nBody content.\n")
	meta, body, _ := parseFrontMatter(raw, "whitespace-skill/SKILL.md")

	// Name should come from path since frontmatter detection fails.
	if meta.Name != "whitespace-skill" {
		t.Errorf("Name = %q, want %q (from path fallback)", meta.Name, "whitespace-skill")
	}
	// Entire content should be the body.
	if body != string([]byte("\n\n---\nname: test\n---\n\nBody content.\n")) {
		t.Errorf("body should be entire content when frontmatter detection fails")
	}
}

func TestSkillParser_EmptyFile(t *testing.T) {
	raw := []byte{}
	meta, body, _ := parseFrontMatter(raw, "empty-skill/SKILL.md")

	if meta.Name != "empty-skill" {
		t.Errorf("Name = %q, want %q (from path)", meta.Name, "empty-skill")
	}
	if body != "" {
		t.Errorf("body = %q, want empty string", body)
	}
}

func TestSkillParser_OnlyFrontMatter(t *testing.T) {
	raw := []byte("---\nname: only-meta\ndescription: Description only\n---\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if meta.Name != "only-meta" {
		t.Errorf("Name = %q, want %q", meta.Name, "only-meta")
	}
	if meta.Description != "Description only" {
		t.Errorf("Description = %q, want %q", meta.Description, "Description only")
	}
	if body != "" {
		t.Errorf("body = %q, want empty string", body)
	}
}

func TestSkillParser_TabsInYAML(t *testing.T) {
	// yaml.v3 handles tabs in values; this should parse successfully.
	raw := []byte("---\nname:\ttest-tabs\ndescription:\tA tabbed description\n---\n\nBody.\n")
	meta, body, warns := parseFrontMatter(raw, "test/SKILL.md")

	// yaml.v3 can handle tabs after the colon in most cases.
	// Check that we get either a valid parse or a warning (no panic).
	if meta.Name == "" && len(warns) == 0 {
		t.Error("expected either a parsed name or a warning, got neither")
	}
	if meta.Name == "test-tabs" {
		// Successful parse — verify description too.
		if meta.Description != "A tabbed description" {
			t.Errorf("Description = %q, want %q", meta.Description, "A tabbed description")
		}
	}
	// Body should always be available.
	if !strings.Contains(body, "Body.") {
		t.Errorf("body = %q, expected to contain 'Body.'", body)
	}
}

func TestSkillParser_DuplicateKeys(t *testing.T) {
	// yaml.v3 (strict mode) treats duplicate keys as an error.
	// parseFrontMatter should fall back to path-derived name with a warning.
	raw := []byte("---\nname: first\nname: second\ndescription: desc\n---\n\nBody.\n")
	meta, body, warns := parseFrontMatter(raw, "dup-skill/SKILL.md")

	// Should have a warning about invalid YAML (duplicate key).
	hasYAMLWarn := false
	for _, w := range warns {
		if strings.Contains(w, "invalid YAML") {
			hasYAMLWarn = true
			break
		}
	}
	if !hasYAMLWarn {
		t.Errorf("expected YAML warning for duplicate keys, got: %v", warns)
	}
	// Name should fall back to path-derived value.
	if meta.Name != "dup-skill" {
		t.Errorf("Name = %q, want %q (fallback from path)", meta.Name, "dup-skill")
	}
	// Body should still be available.
	if !strings.Contains(body, "Body.") {
		t.Errorf("body = %q, expected to contain 'Body.'", body)
	}
}
