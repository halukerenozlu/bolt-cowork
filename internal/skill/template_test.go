package skill

import (
	"strings"
	"testing"
)

func TestGenerateTemplate(t *testing.T) {
	tests := []struct {
		name         string
		meta         SkillMetadata
		wantName     string
		wantDesc     string // non-empty → assert description survives round-trip
		wantVersion  string
		wantAutoTrig bool
		wantBody     bool
	}{
		{
			name: "all fields",
			meta: SkillMetadata{
				Name:        "my-skill",
				Description: "Does something useful",
				Tags:        []string{"tag1", "tag2"},
				Category:    "tools",
				Version:     "2.0.0",
				AutoTrigger: true,
			},
			wantName:     "my-skill",
			wantVersion:  "2.0.0",
			wantAutoTrig: true,
			wantBody:     true,
		},
		{
			name:         "minimal fields defaults applied",
			meta:         SkillMetadata{Name: "minimal"},
			wantName:     "minimal",
			wantVersion:  "1.0.0",
			wantAutoTrig: true,
			wantBody:     true,
		},
		{
			name: "special characters in description",
			meta: SkillMetadata{
				Name:        "my-skill",
				Description: "Review code: security and bugs",
			},
			wantName:     "my-skill",
			wantDesc:     "Review code: security and bugs",
			wantVersion:  "1.0.0",
			wantAutoTrig: true,
			wantBody:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateTemplate(tt.meta)
			if !strings.Contains(got, "name: "+tt.wantName) {
				t.Errorf("output missing name %q:\n%s", tt.wantName, got)
			}
			// Version may be quoted or unquoted depending on yaml encoder.
			if !strings.Contains(got, "version: "+tt.wantVersion) &&
				!strings.Contains(got, `version: "`+tt.wantVersion+`"`) {
				t.Errorf("output missing version %q:\n%s", tt.wantVersion, got)
			}
			if tt.wantAutoTrig && !strings.Contains(got, "auto_trigger: true") {
				t.Errorf("output missing auto_trigger: true:\n%s", got)
			}
			if tt.wantBody && !strings.Contains(got, "[rule 1]") {
				t.Errorf("output missing body placeholder:\n%s", got)
			}
			// Round-trip: verify special characters survive YAML encode/decode.
			if tt.wantDesc != "" {
				parsed, _, warns := parseFrontMatter([]byte(got), "test/SKILL.md")
				if len(warns) != 0 {
					t.Errorf("unexpected warnings parsing generated template: %v", warns)
				}
				if parsed.Description != tt.wantDesc {
					t.Errorf("Description round-trip = %q, want %q", parsed.Description, tt.wantDesc)
				}
			}
		})
	}
}

func TestGenerateTemplate_OutputParseable(t *testing.T) {
	meta := SkillMetadata{
		Name:        "parseable-skill",
		Description: "A skill that can be parsed",
		Tags:        []string{"parse", "test"},
		Category:    "testing",
		Version:     "1.0.0",
		AutoTrigger: true,
	}
	output := GenerateTemplate(meta)

	parsed, _, warns := parseFrontMatter([]byte(output), "test/SKILL.md")
	if len(warns) != 0 {
		t.Errorf("unexpected warnings parsing generated template: %v", warns)
	}
	if parsed.Name != meta.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, meta.Name)
	}
	if parsed.Description != meta.Description {
		t.Errorf("Description = %q, want %q", parsed.Description, meta.Description)
	}
	if parsed.Category != meta.Category {
		t.Errorf("Category = %q, want %q", parsed.Category, meta.Category)
	}
	if parsed.Version != meta.Version {
		t.Errorf("Version = %q, want %q", parsed.Version, meta.Version)
	}
	if !parsed.AutoTrigger {
		t.Error("AutoTrigger = false, want true")
	}
	if len(parsed.Tags) != 2 || parsed.Tags[0] != "parse" || parsed.Tags[1] != "test" {
		t.Errorf("Tags = %v, want [parse test]", parsed.Tags)
	}
}
