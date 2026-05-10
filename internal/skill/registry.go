package skill

import (
	"sort"
	"strings"
)

// SearchByTag returns all skills whose Tags slice contains the given tag
// (case-insensitive exact element match). Returns an empty slice if none found.
func (s *Store) SearchByTag(tag string) []Skill {
	tag = strings.TrimSpace(tag)
	var result []Skill
	for _, sk := range s.skills {
		for _, skTag := range sk.Metadata.Tags {
			if strings.EqualFold(skTag, tag) {
				result = append(result, sk)
				break
			}
		}
	}
	if result == nil {
		return []Skill{}
	}
	return result
}

// ListCategories returns a sorted, deduplicated list of all non-empty Category
// values across all skills. Returns an empty slice if no categories are set.
func (s *Store) ListCategories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, sk := range s.skills {
		if sk.Metadata.Category == "" {
			continue
		}
		key := strings.ToLower(sk.Metadata.Category)
		if seen[key] {
			continue
		}
		seen[key] = true
		cats = append(cats, sk.Metadata.Category)
	}
	sort.Strings(cats)
	if cats == nil {
		return []string{}
	}
	return cats
}

// GetByCategory returns all skills whose Category matches the given value
// (case-insensitive). Returns an empty slice if none found.
func (s *Store) GetByCategory(category string) []Skill {
	var result []Skill
	for _, sk := range s.skills {
		if strings.EqualFold(sk.Metadata.Category, category) {
			result = append(result, sk)
		}
	}
	if result == nil {
		return []Skill{}
	}
	return result
}

// Search returns all skills where the query appears (case-insensitive substring)
// in Name, Description, any Tag, or Category. An empty query matches all skills.
// Results are deduplicated — each skill appears at most once.
// Returns an empty slice if none found.
func (s *Store) Search(query string) []Skill {
	q := strings.ToLower(query)
	var result []Skill
	for _, sk := range s.skills {
		if strings.Contains(strings.ToLower(sk.Metadata.Name), q) ||
			strings.Contains(strings.ToLower(sk.Metadata.Description), q) ||
			strings.Contains(strings.ToLower(sk.Metadata.Category), q) {
			result = append(result, sk)
			continue
		}
		for _, tag := range sk.Metadata.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				result = append(result, sk)
				break
			}
		}
	}
	if result == nil {
		return []Skill{}
	}
	return result
}
