package skill

import "errors"

// ErrSkillNotFound is returned when a skill with the given name does not exist in the store.
var ErrSkillNotFound = errors.New("skill not found")

// Skill represents a loaded skill parsed from a SKILL.md file.
type Skill struct {
	Name        string
	Description string
	AutoTrigger bool
	Content     string // Markdown body (excluding frontmatter)
	Source      string // "global" or "local"
	FilePath    string // Absolute path to the SKILL.md file
}

// SkillStore defines the interface for loading and querying skills.
type SkillStore interface {
	LoadAll(dirs []string) error
	GetAll() []Skill
	GetByName(name string) (*Skill, error)
}

// Store is the default in-memory implementation of SkillStore.
type Store struct {
	skills []Skill
}

// NewStore creates a new empty Store.
func NewStore() *Store {
	return &Store{}
}
