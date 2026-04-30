package skill

import (
	"strings"
	"testing"
)

func TestBuildSkillContext(t *testing.T) {
	tests := []struct {
		name         string
		skills       []Skill
		wantEmpty    bool
		wantContains []string
		wantPrefix   string
		wantSuffix   string
		wantOrder    []string // first must appear before second
	}{
		{
			name:      "empty slice",
			skills:    []Skill{},
			wantEmpty: true,
		},
		{
			name:         "single skill",
			skills:       []Skill{{Metadata: SkillMetadata{Name: "file-organizer"}, Content: "Organize files into subdirectories by extension."}},
			wantContains: []string{"## Skill: file-organizer", "Organize files into subdirectories by extension."},
		},
		{
			name: "multiple skills",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "file-organizer"}, Content: "Organize files."},
				{Metadata: SkillMetadata{Name: "summarizer"}, Content: "Summarize documents."},
			},
			wantContains: []string{"## Skill: file-organizer", "## Skill: summarizer"},
			wantOrder:    []string{"file-organizer", "summarizer"},
		},
		{
			name:       "contains XML tags",
			skills:     []Skill{{Metadata: SkillMetadata{Name: "s"}, Content: "c"}},
			wantPrefix: "<active_skills>",
			wantSuffix: "</active_skills>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSkillContext(tt.skills)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
				return
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("result missing %q", s)
				}
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("expected prefix %q", tt.wantPrefix)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("expected suffix %q", tt.wantSuffix)
			}
			if len(tt.wantOrder) == 2 {
				if strings.Index(got, tt.wantOrder[0]) > strings.Index(got, tt.wantOrder[1]) {
					t.Errorf("%q should appear before %q", tt.wantOrder[0], tt.wantOrder[1])
				}
			}
		})
	}
}

func TestInjectSkills(t *testing.T) {
	basePrompt := "You are a file operations planner."

	tests := []struct {
		name          string
		skills        []Skill
		wantUnchanged bool
		wantContains  []string
		wantHasPrefix string
	}{
		{
			name:          "empty skills returns prompt unchanged",
			skills:        []Skill{},
			wantUnchanged: true,
		},
		{
			name:          "appends skill context",
			skills:        []Skill{{Metadata: SkillMetadata{Name: "file-organizer"}, Content: "Organize files."}},
			wantContains:  []string{"<active_skills>"},
			wantHasPrefix: basePrompt,
		},
		{
			name:          "preserves original with separator",
			skills:        []Skill{{Metadata: SkillMetadata{Name: "s"}, Content: "c"}},
			wantHasPrefix: basePrompt + "\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectSkills(basePrompt, tt.skills)
			if tt.wantUnchanged {
				if got != basePrompt {
					t.Fatalf("expected prompt unchanged, got %q", got)
				}
				return
			}
			if tt.wantHasPrefix != "" && !strings.HasPrefix(got, tt.wantHasPrefix) {
				t.Errorf("expected prefix %q, got %q", tt.wantHasPrefix, got[:min(len(got), len(tt.wantHasPrefix)+5)])
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("result missing %q", s)
				}
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
