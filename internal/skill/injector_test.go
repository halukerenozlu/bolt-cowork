package skill

import (
	"strings"
	"testing"
)

func TestBuildSkillContext_Empty(t *testing.T) {
	got := BuildSkillContext([]Skill{})
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestBuildSkillContext_Single(t *testing.T) {
	skills := []Skill{
		{Name: "file-organizer", Content: "Organize files into subdirectories by extension."},
	}
	got := BuildSkillContext(skills)
	if !strings.Contains(got, "## Skill: file-organizer") {
		t.Error("missing skill header")
	}
	if !strings.Contains(got, "Organize files into subdirectories by extension.") {
		t.Error("missing skill content")
	}
}

func TestBuildSkillContext_Multiple(t *testing.T) {
	skills := []Skill{
		{Name: "file-organizer", Content: "Organize files."},
		{Name: "summarizer", Content: "Summarize documents."},
	}
	got := BuildSkillContext(skills)
	if !strings.Contains(got, "## Skill: file-organizer") {
		t.Error("missing file-organizer header")
	}
	if !strings.Contains(got, "## Skill: summarizer") {
		t.Error("missing summarizer header")
	}
	// file-organizer must appear before summarizer
	if strings.Index(got, "file-organizer") > strings.Index(got, "summarizer") {
		t.Error("skills not in order")
	}
}

func TestBuildSkillContext_ContainsXMLTags(t *testing.T) {
	skills := []Skill{{Name: "s", Content: "c"}}
	got := BuildSkillContext(skills)
	if !strings.HasPrefix(got, "<active_skills>") {
		t.Error("missing opening <active_skills> tag")
	}
	if !strings.HasSuffix(got, "</active_skills>") {
		t.Error("missing closing </active_skills> tag")
	}
}

func TestInjectSkills_Empty(t *testing.T) {
	prompt := "You are a file operations planner."
	got := InjectSkills(prompt, []Skill{})
	if got != prompt {
		t.Fatalf("expected prompt unchanged, got %q", got)
	}
}

func TestInjectSkills_Appends(t *testing.T) {
	prompt := "You are a file operations planner."
	skills := []Skill{{Name: "file-organizer", Content: "Organize files."}}
	got := InjectSkills(prompt, skills)
	if !strings.HasPrefix(got, prompt) {
		t.Error("original prompt not preserved at start")
	}
	if !strings.Contains(got, "<active_skills>") {
		t.Error("skill context not appended")
	}
}

func TestInjectSkills_PreservesOriginal(t *testing.T) {
	prompt := "You are a file operations planner."
	skills := []Skill{{Name: "s", Content: "c"}}
	got := InjectSkills(prompt, skills)
	if !strings.HasPrefix(got, prompt) {
		t.Fatalf("original prompt was modified: %q", got)
	}
	// Verify the separator is exactly "\n\n"
	expected := prompt + "\n\n"
	if !strings.HasPrefix(got, expected) {
		t.Errorf("expected %q prefix, got %q", expected, got[:min(len(got), len(expected)+5)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
