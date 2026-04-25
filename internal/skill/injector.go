package skill

import "strings"

// BuildSkillContext formats a slice of skills into an XML-tagged block
// suitable for injection into a planner system prompt. Returns an empty
// string if skills is empty.
func BuildSkillContext(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var parts []string
	for _, sk := range skills {
		parts = append(parts, "## Skill: "+sk.Name+"\n"+sk.Content)
	}
	return "<active_skills>\n" + strings.Join(parts, "\n\n") + "\n</active_skills>"
}

// InjectSkills appends the skill context block to systemPrompt and returns
// the combined string. If skills is empty, systemPrompt is returned unchanged.
func InjectSkills(systemPrompt string, skills []Skill) string {
	ctx := BuildSkillContext(skills)
	if ctx == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n" + ctx
}
