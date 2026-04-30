package prompt

import (
	"fmt"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/internal/tool"
)

// ToolDescription is a prompt-friendly representation of a tool.
type ToolDescription struct {
	Name        string
	Description string
	ReadOnly    bool
	Destructive bool
}

// SkillContext holds a skill's name and content for prompt injection.
type SkillContext struct {
	Name    string
	Content string
}

// BuildOptions configures what sections the PromptBuilder includes.
type BuildOptions struct {
	Tools      []ToolDescription
	Skills     []SkillContext
	ProjectCtx string // e.g. .cowork/context.md contents
}

// PromptBuilder assembles a system prompt from a base template plus
// optional tool descriptions, skill contexts, and project context.
type PromptBuilder struct {
	basePrompt string
}

// NewPromptBuilder creates a PromptBuilder with the given base prompt text.
func NewPromptBuilder(base string) *PromptBuilder {
	return &PromptBuilder{basePrompt: base}
}

// Build concatenates the base prompt with tool descriptions, skill contexts,
// and project context as configured in opts.
func (b *PromptBuilder) Build(opts BuildOptions) string {
	var sb strings.Builder
	sb.WriteString(b.basePrompt)

	if len(opts.Tools) > 0 {
		sb.WriteString("\n\n## Available Tools\n")
		for _, t := range opts.Tools {
			tag := ""
			if t.Destructive {
				tag = " [destructive]"
			} else if t.ReadOnly {
				tag = " [read-only]"
			}
			sb.WriteString(fmt.Sprintf("- %s: %s%s\n", t.Name, t.Description, tag))
		}
	}

	if len(opts.Skills) > 0 {
		sb.WriteString("\n## Skills\n")
		for _, s := range opts.Skills {
			sb.WriteString(fmt.Sprintf("### %s\n%s\n", s.Name, s.Content))
		}
	}

	if opts.ProjectCtx != "" {
		sb.WriteString("\n## Project Context\n")
		sb.WriteString(opts.ProjectCtx)
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToolDescriptionsFromRegistry converts all tools in a tool.Registry into
// prompt-friendly ToolDescription values.
func ToolDescriptionsFromRegistry(reg *tool.Registry) []ToolDescription {
	tools := reg.All()
	descs := make([]ToolDescription, len(tools))
	for i, t := range tools {
		descs[i] = ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			ReadOnly:    t.IsReadOnly(),
			Destructive: t.IsDestructive(),
		}
	}
	return descs
}

// SkillContextsFromStore converts a slice of skills into SkillContext values.
func SkillContextsFromStore(skills []skill.Skill) []SkillContext {
	ctxs := make([]SkillContext, len(skills))
	for i, sk := range skills {
		ctxs[i] = SkillContext{
			Name:    sk.Name,
			Content: sk.Content,
		}
	}
	return ctxs
}
