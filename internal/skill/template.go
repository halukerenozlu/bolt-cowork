package skill

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter is used to safely marshal skill metadata to YAML,
// ensuring colons, quotes, and other special characters in user-supplied
// strings are properly escaped by the YAML encoder.
type skillFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Category    string   `yaml:"category"`
	Version     string   `yaml:"version"`
	AutoTrigger bool     `yaml:"auto_trigger"`
}

// GenerateTemplate returns a complete SKILL.md string for the given metadata.
// Defaults applied when fields are zero-valued:
//   - Version: "" → "1.0.0"
//   - AutoTrigger: always set to true in the generated template
//   - Tags: nil → empty slice (renders as [] in YAML)
func GenerateTemplate(meta SkillMetadata) string {
	if meta.Version == "" {
		meta.Version = "1.0.0"
	}
	tags := meta.Tags
	if tags == nil {
		tags = []string{}
	}

	fm := skillFrontmatter{
		Name:        meta.Name,
		Description: meta.Description,
		Tags:        tags,
		Category:    meta.Category,
		Version:     meta.Version,
		AutoTrigger: true,
	}

	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		// Fallback: should never occur for these value types.
		return fmt.Sprintf("---\nname: %s\n---\n", meta.Name)
	}

	return "---\n" + string(yamlBytes) + "---\n" +
		"When this skill is active, [describe what the agent should do].\n\n" +
		"Follow these rules:\n" +
		"- [rule 1]\n" +
		"- [rule 2]\n" +
		"- [rule 3]\n"
}
