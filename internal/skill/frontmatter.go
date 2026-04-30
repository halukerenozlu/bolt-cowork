package skill

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseFrontMatter splits a SKILL.md file into its YAML frontmatter metadata
// and Markdown body. It applies fallback logic for missing fields:
//   - No front matter → name from filename (if path provided), description from first paragraph
//   - Missing name → derive from filename
//   - Missing description → first paragraph (max 512 chars)
//   - Invalid YAML → warning returned, fallback values used, content still loaded
//
// The path parameter is used only for fallback name derivation and may be empty.
func parseFrontMatter(raw []byte, path string) (SkillMetadata, string, []string) {
	// Normalize CRLF → LF so delimiter detection works on Windows files.
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	content := string(raw)
	var warnings []string

	// No frontmatter delimiter — return entire content as body with fallback metadata.
	if !strings.HasPrefix(content, "---\n") {
		meta := SkillMetadata{
			Name:        nameFromPath(path),
			Description: descriptionFallback(content, 512),
		}
		if meta.Name == "" {
			warnings = append(warnings, "skill: no frontmatter and cannot derive name from path")
		}
		return meta, content, warnings
	}

	rest := content[4:] // skip opening "---\n"
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		// Unterminated frontmatter — treat entire content as body.
		meta := SkillMetadata{
			Name:        nameFromPath(path),
			Description: descriptionFallback(content, 512),
		}
		warnings = append(warnings, fmt.Sprintf("skill: unterminated frontmatter in %q, using fallback values", path))
		return meta, content, warnings
	}

	yamlPart := rest[:idx]
	// Body starts after "\n---" (4 chars); skip optional following newline.
	body := rest[idx+4:]
	body = strings.TrimPrefix(body, "\n")

	var meta SkillMetadata
	if err := yaml.Unmarshal([]byte(yamlPart), &meta); err != nil {
		meta = SkillMetadata{
			Name:        nameFromPath(path),
			Description: descriptionFallback(body, 512),
		}
		warnings = append(warnings, fmt.Sprintf("skill: invalid YAML in %q: %v, using fallback values", path, err))
		return meta, body, warnings
	}

	// Fallback for missing name.
	if meta.Name == "" {
		meta.Name = nameFromPath(path)
		if meta.Name != "" {
			warnings = append(warnings, fmt.Sprintf("skill: %q missing name field, derived %q from filename", path, meta.Name))
		}
	}

	// Fallback for missing description.
	if meta.Description == "" {
		meta.Description = descriptionFallback(body, 512)
	}

	return meta, body, warnings
}

// descriptionFallback extracts the first non-empty paragraph from content,
// truncated to maxLen characters. Returns empty string if content is empty.
func descriptionFallback(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	var para strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if para.Len() > 0 {
				break
			}
			continue
		}
		// Skip markdown headings for description.
		if strings.HasPrefix(trimmed, "#") {
			if para.Len() > 0 {
				break
			}
			continue
		}
		if para.Len() > 0 {
			para.WriteString(" ")
		}
		para.WriteString(trimmed)
	}
	result := para.String()
	if len(result) > maxLen {
		result = result[:maxLen]
	}
	return result
}

// nameFromPath derives a skill name from the file path by stripping the
// extension from the filename. For "SKILL.md" files, the parent directory
// name is used instead (e.g., "file-organizer/SKILL.md" → "file-organizer").
// Returns empty string if path is empty.
func nameFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	// For SKILL.md files, use parent directory name.
	if strings.EqualFold(base, "SKILL.md") {
		dir := filepath.Dir(path)
		parent := filepath.Base(dir)
		// Avoid returning "." or filesystem root.
		if parent == "." || parent == string(filepath.Separator) {
			return ""
		}
		return parent
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
