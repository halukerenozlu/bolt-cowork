package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ParseFile reads and parses a SKILL.md file. It returns a *Skill with all
// fields populated except Scope (set by LoadAll/LoadEmbedded). Front matter
// parsing uses fallback logic: missing name is derived from the filename,
// missing description from the first paragraph.
func ParseFile(path string) (*Skill, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: read %q: %w", path, err)
	}

	meta, body, warnings := parseFrontMatter(data, path)

	if meta.Name == "" {
		return nil, warnings, fmt.Errorf("skill: %q: name is required (not in frontmatter and cannot derive from path)", path)
	}

	return &Skill{
		Metadata: meta,
		Content:  body,
		FilePath: path,
	}, warnings, nil
}

// LoadAll loads all SKILL.md files found (recursively) in each of the given
// directories in order. Later entries override earlier ones when two skills
// share the same name. It never returns a hard error; all issues are returned
// as human-readable warning strings. Scope is set to ScopeGlobal if the
// directory is under the user's home directory, and ScopeProject otherwise.
func (s *Store) LoadAll(dirs []string) []string {
	home, _ := os.UserHomeDir()
	var warnings []string

	for _, dir := range dirs {
		scope := ScopeProject
		isUnderHome := home != "" && strings.HasPrefix(dir, home)
		if isUnderHome {
			scope = ScopeGlobal
		}

		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}

			// Check for empty file before parsing.
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				warnings = append(warnings, fmt.Sprintf("Warning: failed to read skill file %q: %v", path, readErr))
				return nil
			}
			if len(strings.TrimSpace(string(raw))) == 0 {
				warnings = append(warnings, fmt.Sprintf("Warning: skill file %q has no content, skipping", path))
				return nil
			}

			sk, parseWarns, parseErr := ParseFile(path)
			warnings = append(warnings, parseWarns...)
			if parseErr != nil {
				warnings = append(warnings, fmt.Sprintf("Warning: failed to parse skill %q: %v", path, parseErr))
				return nil
			}
			sk.Scope = scope

			// Warn on name conflict before overriding.
			if existing, err := s.GetByName(sk.Metadata.Name); err == nil {
				warnings = append(warnings, fmt.Sprintf("Skill %q overridden by %s version (was: %s)", sk.Metadata.Name, scope, existing.Scope))
			}
			s.Upsert(sk)
			return nil
		})

		if err != nil {
			if os.IsNotExist(err) {
				if isUnderHome {
					warnings = append(warnings, fmt.Sprintf("Info: global skills directory not found, skipping: %s", dir))
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Warning: skill: walk %q: %v", dir, err))
		}
	}
	return warnings
}

// LoadEmbedded loads skills from an embedded fs.FS. Skills loaded this way
// have Scope set to ScopeBundled. Invalid or malformed files are silently
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
		meta, body, _ := parseFrontMatter(data, path)
		if meta.Name == "" {
			return nil
		}
		s.Upsert(&Skill{
			Metadata: meta,
			Scope:    ScopeBundled,
			Content:  body,
			FilePath: path,
		})
		return nil
	})
}

// Upsert adds a skill to the store. If a skill with the same name already
// exists it is replaced (last-write-wins for override semantics).
func (s *Store) Upsert(skill *Skill) {
	for i, existing := range s.skills {
		if existing.Metadata.Name == skill.Metadata.Name {
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
		if s.skills[i].Metadata.Name == name {
			skill := s.skills[i]
			return &skill, nil
		}
	}
	return nil, ErrSkillNotFound
}
