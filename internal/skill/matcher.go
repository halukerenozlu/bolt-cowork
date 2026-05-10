package skill

import (
	"sort"
	"strings"
)

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

// skillTokens returns the merged, deduplicated token set for a skill by
// combining tokenized Description tokens with lowercased Tags.
func skillTokens(sk Skill) []string {
	tokens := tokenize(sk.Metadata.Description)
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		seen[t] = true
	}
	for _, tag := range sk.Metadata.Tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		t := strings.ToLower(trimmed)
		if seen[t] {
			continue
		}
		seen[t] = true
		tokens = append(tokens, t)
	}
	return tokens
}

// MatchScored returns scored match results for all auto-trigger skills whose
// tokens appear in the command. Score is the ratio of matched tokens to total
// skill tokens. Strength is "strong" when score >= 0.3 and at least 2 tokens
// matched, "weak" otherwise. Results are sorted by score descending.
func (s *Store) MatchScored(command string) []MatchResult {
	cmd := strings.ToLower(command)
	var results []MatchResult
	for _, sk := range s.skills {
		if !sk.Metadata.AutoTrigger {
			continue
		}
		tokens := skillTokens(sk)
		if len(tokens) == 0 {
			continue
		}
		matched := 0
		for _, tok := range tokens {
			if strings.Contains(cmd, tok) {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		score := float64(matched) / float64(len(tokens))
		strength := "weak"
		if score >= 0.3 && matched >= 2 {
			strength = "strong"
		}
		results = append(results, MatchResult{Skill: sk, Score: score, Strength: strength})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Skill.Metadata.Name < results[j].Skill.Metadata.Name
	})
	return results
}

// MatchHybrid returns the best skill matches using a three-layer strategy:
//  1. Run MatchScored to get scored candidates.
//  2. If exactly one strong match exists, return it without calling the LLM.
//  3. If multiple strong matches or only weak matches exist, call
//     llm.Disambiguate to narrow the candidates. If llm is nil or
//     Disambiguate returns an error, all scored results are returned as-is.
func (s *Store) MatchHybrid(command string, llm LLMDisambiguator) []MatchResult {
	results := s.MatchScored(command)
	if len(results) == 0 {
		return results
	}

	var strong []MatchResult
	for _, r := range results {
		if r.Strength == "strong" {
			strong = append(strong, r)
		}
	}

	if len(strong) == 1 {
		return strong
	}

	if llm == nil {
		return results
	}

	candidates := make([]Skill, len(results))
	for i, r := range results {
		candidates[i] = r.Skill
	}
	disambiguated, err := llm.Disambiguate(command, candidates)
	if err != nil {
		return results
	}

	scoreMap := make(map[string]MatchResult, len(results))
	for _, r := range results {
		scoreMap[r.Skill.Metadata.Name] = r
	}
	var out []MatchResult
	for _, sk := range disambiguated {
		if r, ok := scoreMap[sk.Metadata.Name]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Match returns all auto-trigger skills whose description keywords appear in
// the given command string. Matching is case-insensitive and uses substring
// containment (strings.Contains). Skills with AutoTrigger=false are skipped.
func (s *Store) Match(command string) []Skill {
	results := s.MatchScored(command)
	if len(results) == 0 {
		return []Skill{}
	}
	skills := make([]Skill, len(results))
	for i, r := range results {
		skills[i] = r.Skill
	}
	return skills
}
