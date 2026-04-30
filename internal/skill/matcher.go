package skill

import "strings"

// stopWords is the set of common words ignored during keyword matching.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"to": true, "in": true, "on": true, "of": true, "and": true,
	"or": true, "for": true, "with": true, "by": true, "from": true,
	"this": true, "that": true, "it": true,
}

// tokenize splits text into unique, lowercase tokens with stop words removed.
func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool, len(words))
	var tokens []string
	for _, w := range words {
		if stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		tokens = append(tokens, w)
	}
	return tokens
}

// Match returns all auto-trigger skills whose description keywords appear in
// the given command string. Matching is case-insensitive and uses substring
// containment (strings.Contains). Skills with AutoTrigger=false are skipped.
func (s *Store) Match(command string) []Skill {
	cmd := strings.ToLower(command)
	var matched []Skill
	for _, sk := range s.skills {
		if !sk.Metadata.AutoTrigger {
			continue
		}
		tokens := tokenize(sk.Metadata.Description)
		if len(tokens) == 0 {
			continue
		}
		for _, tok := range tokens {
			if strings.Contains(cmd, tok) {
				matched = append(matched, sk)
				break
			}
		}
	}
	if matched == nil {
		return []Skill{}
	}
	return matched
}
