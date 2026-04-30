package prompt

import (
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/skill"
)

func TestNewPromptBuilder(t *testing.T) {
	b := NewPromptBuilder("base")
	if b.basePrompt != "base" {
		t.Fatalf("expected base prompt 'base', got %q", b.basePrompt)
	}
}

func TestBuild_BaseOnly(t *testing.T) {
	b := NewPromptBuilder("You are a helpful assistant.")
	got := b.Build(BuildOptions{})
	if got != "You are a helpful assistant." {
		t.Fatalf("expected base only, got %q", got)
	}
}

func TestBuild_WithTools(t *testing.T) {
	b := NewPromptBuilder("Base prompt.")
	got := b.Build(BuildOptions{
		Tools: []ToolDescription{
			{Name: "read", Description: "Read a file", ReadOnly: true},
			{Name: "delete", Description: "Delete a file", Destructive: true},
			{Name: "write", Description: "Write a file"},
		},
	})

	if !strings.Contains(got, "## Available Tools") {
		t.Fatal("missing Available Tools section")
	}
	if !strings.Contains(got, "- read: Read a file [read-only]") {
		t.Fatal("missing read tool with [read-only] tag")
	}
	if !strings.Contains(got, "- delete: Delete a file [destructive]") {
		t.Fatal("missing delete tool with [destructive] tag")
	}
	if !strings.Contains(got, "- write: Write a file\n") {
		t.Fatal("missing write tool without tag")
	}
}

func TestBuild_WithSkills(t *testing.T) {
	b := NewPromptBuilder("Base.")
	got := b.Build(BuildOptions{
		Skills: []SkillContext{
			{Name: "file-organizer", Content: "Organize files by type."},
		},
	})

	if !strings.Contains(got, "## Skills") {
		t.Fatal("missing Skills section")
	}
	if !strings.Contains(got, "### file-organizer") {
		t.Fatal("missing skill name header")
	}
	if !strings.Contains(got, "Organize files by type.") {
		t.Fatal("missing skill content")
	}
}

func TestBuild_WithProjectCtx(t *testing.T) {
	b := NewPromptBuilder("Base.")
	got := b.Build(BuildOptions{
		ProjectCtx: "This is a Go project.",
	})

	if !strings.Contains(got, "## Project Context") {
		t.Fatal("missing Project Context section")
	}
	if !strings.Contains(got, "This is a Go project.") {
		t.Fatal("missing project context content")
	}
}

func TestBuild_AllSections(t *testing.T) {
	b := NewPromptBuilder("System base.")
	got := b.Build(BuildOptions{
		Tools: []ToolDescription{
			{Name: "list", Description: "List directory"},
		},
		Skills: []SkillContext{
			{Name: "sorter", Content: "Sort items."},
		},
		ProjectCtx: "Go project context.",
	})

	// Verify ordering: base → tools → skills → project
	toolsIdx := strings.Index(got, "## Available Tools")
	skillsIdx := strings.Index(got, "## Skills")
	projIdx := strings.Index(got, "## Project Context")

	if toolsIdx == -1 || skillsIdx == -1 || projIdx == -1 {
		t.Fatal("one or more sections missing")
	}
	if toolsIdx >= skillsIdx {
		t.Fatal("tools section should come before skills")
	}
	if skillsIdx >= projIdx {
		t.Fatal("skills section should come before project context")
	}
}

func TestSkillContextsFromStore(t *testing.T) {
	skills := []skill.Skill{
		{Metadata: skill.SkillMetadata{Name: "s1", Description: "desc1"}, Content: "body1"},
		{Metadata: skill.SkillMetadata{Name: "s2", Description: "desc2"}, Content: "body2"},
	}

	ctxs := SkillContextsFromStore(skills)
	if len(ctxs) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(ctxs))
	}
	if ctxs[0].Name != "s1" || ctxs[0].Content != "body1" {
		t.Fatalf("unexpected first context: %+v", ctxs[0])
	}
	if ctxs[1].Name != "s2" || ctxs[1].Content != "body2" {
		t.Fatalf("unexpected second context: %+v", ctxs[1])
	}
}

func TestSkillContextsFromStore_Empty(t *testing.T) {
	ctxs := SkillContextsFromStore(nil)
	if len(ctxs) != 0 {
		t.Fatalf("expected 0 contexts for nil input, got %d", len(ctxs))
	}
}
