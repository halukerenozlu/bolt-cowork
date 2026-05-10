package skill

import "errors"

// ErrSkillNotFound is returned when a skill with the given name does not exist in the store.
var ErrSkillNotFound = errors.New("skill not found")

// SkillScope represents where a skill was loaded from.
type SkillScope int

const (
	// ScopeBundled indicates a skill embedded in the binary.
	ScopeBundled SkillScope = iota
	// ScopeGlobal indicates a skill from ~/.bolt-cowork/skills/.
	ScopeGlobal
	// ScopeProject indicates a skill from the project-local ./bolt-skills/ directory.
	ScopeProject
)

// String returns a human-readable label for the scope.
func (s SkillScope) String() string {
	switch s {
	case ScopeBundled:
		return "bundled"
	case ScopeGlobal:
		return "global"
	case ScopeProject:
		return "project"
	default:
		return "unknown"
	}
}

// SkillMetadata holds the parsed YAML frontmatter fields of a SKILL.md file.
type SkillMetadata struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Tags             []string `yaml:"tags"`
	Category         string   `yaml:"category"`
	Version          string   `yaml:"version"`
	Priority         int      `yaml:"priority"`
	AutoTrigger      bool     `yaml:"auto_trigger"`
	RequiresApproval bool     `yaml:"requires_approval"`
}

// Skill represents a loaded skill parsed from a SKILL.md file.
type Skill struct {
	Metadata SkillMetadata
	Scope    SkillScope
	Content  string // Markdown body (excluding frontmatter)
	FilePath string // Absolute path to the SKILL.md file
}

// SkillStore defines the interface for loading and querying skills.
type SkillStore interface {
	LoadAll(dirs []string) []string
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
