package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterFields holds the YAML frontmatter fields of a SKILL.md file.
type frontmatterFields struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	AutoTrigger bool   `yaml:"auto_trigger"`
}

// parseFrontmatter splits a SKILL.md file content into its YAML frontmatter and
// Markdown body. The file must start with "---\n". Returns an error if the
// frontmatter delimiters are missing or malformed.
func parseFrontmatter(content string) (yamlPart, body string, err error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", "", fmt.Errorf("skill: no YAML frontmatter found (file must start with ---)")
	}
	rest := content[4:] // skip opening "---\n"
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", "", fmt.Errorf("skill: unterminated frontmatter (closing --- not found)")
	}
	yamlPart = rest[:idx]
	// body starts after "\n---" which is 4 chars; skip an optional following newline
	after := rest[idx+4:]
	after = strings.TrimPrefix(after, "\n")
	return yamlPart, after, nil
}

// ParseFile reads and parses a SKILL.md file. It returns a *Skill with all
// fields populated except Source (set by LoadAll). An error is returned if the
// file cannot be read, has no frontmatter, or has an empty name field.
func ParseFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skill: read %q: %w", path, err)
	}

	yamlPart, body, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("skill: parse %q: %w", path, err)
	}

	var fm frontmatterFields
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return nil, fmt.Errorf("skill: parse frontmatter %q: %w", path, err)
	}

	if fm.Name == "" {
		return nil, fmt.Errorf("skill: %q: name is required", path)
	}

	return &Skill{
		Name:        fm.Name,
		Description: fm.Description,
		AutoTrigger: fm.AutoTrigger,
		Content:     body,
		FilePath:    path,
	}, nil
}

// LoadAll loads all SKILL.md files found (recursively) in each of the given
// directories in order. Later entries override earlier ones when two skills
// share the same name. Directories that do not exist are silently skipped.
// Source is set to "global" if the directory is under the user's home directory,
// and "local" otherwise.
func (s *Store) LoadAll(dirs []string) error {
	home, _ := os.UserHomeDir()

	for _, dir := range dirs {
		source := "local"
		if home != "" && strings.HasPrefix(dir, home) {
			source = "global"
		}

		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}
			skill, err := ParseFile(path)
			if err != nil {
				// Skip unparseable files silently.
				return nil
			}
			skill.Source = source
			s.Upsert(skill)
			return nil
		})

		if err != nil {
			if os.IsNotExist(err) {
				continue // directory does not exist — skip
			}
			return fmt.Errorf("skill: walk %q: %w", dir, err)
		}
	}
	return nil
}

// LoadEmbedded loads skills from an embedded fs.FS. Skills loaded this way
// have Source set to "bundled". Invalid or malformed files are silently
// skipped. Call LoadEmbedded before LoadAll so that filesystem skills can
// override bundled defaults.
func (s *Store) LoadEmbedded(fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil
		}
		yamlPart, body, err := parseFrontmatter(string(data))
		if err != nil {
			return nil
		}
		var fm frontmatterFields
		if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
			return nil
		}
		if fm.Name == "" {
			return nil
		}
		s.Upsert(&Skill{
			Name:        fm.Name,
			Description: fm.Description,
			AutoTrigger: fm.AutoTrigger,
			Content:     body,
			Source:      "bundled",
			FilePath:    path,
		})
		return nil
	})
}

// Upsert adds a skill to the store. If a skill with the same name already
// exists it is replaced (last-write-wins for override semantics).
func (s *Store) Upsert(skill *Skill) {
	for i, existing := range s.skills {
		if existing.Name == skill.Name {
			s.skills[i] = *skill
			return
		}
	}
	s.skills = append(s.skills, *skill)
}

// GetAll returns a copy of all skills in the store.
func (s *Store) GetAll() []Skill {
	result := make([]Skill, len(s.skills))
	copy(result, s.skills)
	return result
}

// GetByName returns the skill with the given name, or ErrSkillNotFound if none
// exists.
func (s *Store) GetByName(name string) (*Skill, error) {
	for i := range s.skills {
		if s.skills[i].Name == name {
			skill := s.skills[i]
			return &skill, nil
		}
	}
	return nil, ErrSkillNotFound
}
