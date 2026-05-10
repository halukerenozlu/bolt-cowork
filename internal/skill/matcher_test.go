package skill

import (
	"testing"
)

// mockDisambiguator records whether Disambiguate was called and returns a
// preset result for testing MatchHybrid LLM interaction.
type mockDisambiguator struct {
	called bool
	result []Skill
	err    error
}

func (m *mockDisambiguator) Disambiguate(_ string, _ []Skill) ([]Skill, error) {
	m.called = true
	return m.result, m.err
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"basic", "Organizes files by type", []string{"organizes", "files", "type"}},
		{"all stop words", "the a an is are", nil},
		{"duplicates removed", "file file file", []string{"file"}},
		{"lowercase", "HELLO World", []string{"hello", "world"}},
		{"empty string", "", nil},
		{"mixed stop and real", "to organize the files", []string{"organize", "files"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("tokenize(%q) = %v, want %v", tt.text, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.text, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatch_SingleKeyword(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true}})

	got := s.Match("organize files")
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Metadata.Name != "file-organizer" {
		t.Errorf("expected file-organizer, got %s", got[0].Metadata.Name)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-organizer", Description: "Organizes files", AutoTrigger: true}})

	got := s.Match("delete all")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(got))
	}
}

func TestMatch_MultipleSkills(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true}})
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-cleaner", Description: "Cleans old files", AutoTrigger: true}})

	got := s.Match("clean and organize files")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true}})

	upper := s.Match("ORGANIZE FILES")
	lower := s.Match("organize files")
	if len(upper) != len(lower) {
		t.Fatalf("case mismatch: upper=%d lower=%d", len(upper), len(lower))
	}
	if len(upper) != 1 {
		t.Fatalf("expected 1 match, got %d", len(upper))
	}
}

func TestMatch_AutoTriggerOnly(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "manual-skill", Description: "Organizes files", AutoTrigger: false}})

	got := s.Match("organize files")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for AutoTrigger=false, got %d", len(got))
	}
}

func TestMatch_StopWordsIgnored(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "stop-only", Description: "the a an is are", AutoTrigger: true}})

	got := s.Match("the a an is are")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for stop-words-only description, got %d", len(got))
	}
}

func TestMatch_EmptyCommand(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "file-organizer", Description: "Organizes files", AutoTrigger: true}})

	got := s.Match("")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for empty command, got %d", len(got))
	}
}

func TestSkillTokens(t *testing.T) {
	tests := []struct {
		name      string
		descr     string
		tags      []string
		wantAll   []string // all of these must be present
		wantNoDup bool     // always checked
	}{
		{
			name:      "description only",
			descr:     "organize files",
			tags:      nil,
			wantAll:   []string{"organize", "files"},
			wantNoDup: true,
		},
		{
			name:      "includes tags",
			descr:     "organize files",
			tags:      []string{"sort", "tidy"},
			wantAll:   []string{"organize", "files", "sort", "tidy"},
			wantNoDup: true,
		},
		{
			name:      "no duplicates between description and tags",
			descr:     "sort organize files",
			tags:      []string{"sort", "tidy"},
			wantAll:   []string{"sort", "organize", "files", "tidy"},
			wantNoDup: true,
		},
		{
			name:      "tags lowercased",
			descr:     "organize files",
			tags:      []string{"SORT", "Tidy"},
			wantAll:   []string{"organize", "files", "sort", "tidy"},
			wantNoDup: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sk := Skill{Metadata: SkillMetadata{Description: tt.descr, Tags: tt.tags}}
			got := skillTokens(sk)
			gotSet := make(map[string]bool, len(got))
			for _, tok := range got {
				gotSet[tok] = true
			}
			for _, want := range tt.wantAll {
				if !gotSet[want] {
					t.Errorf("token %q missing from result %v", want, got)
				}
			}
			if tt.wantNoDup {
				seen := make(map[string]bool, len(got))
				for _, tok := range got {
					if seen[tok] {
						t.Errorf("duplicate token %q in result %v", tok, got)
					}
					seen[tok] = true
				}
			}
		})
	}
}

func TestMatchScored(t *testing.T) {
	tests := []struct {
		name         string
		descr        string
		command      string
		wantLen      int
		wantScore    float64
		wantStrength string
	}{
		{
			name:         "four tokens two matched is strong",
			descr:        "sort group organize files",
			command:      "sort group documents",
			wantLen:      1,
			wantScore:    0.5,
			wantStrength: "strong",
		},
		{
			name:         "six tokens one matched is weak",
			descr:        "sort group organize compress archive backup",
			command:      "sort everything",
			wantLen:      1,
			wantStrength: "weak",
		},
		{
			name:    "no match returns empty",
			descr:   "sort group organize files",
			command: "delete all documents",
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			s.Upsert(&Skill{Metadata: SkillMetadata{
				Name: "test-skill", Description: tt.descr, AutoTrigger: true,
			}})
			got := s.MatchScored(tt.command)
			if len(got) != tt.wantLen {
				t.Fatalf("len(results) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			if tt.wantStrength != "" && got[0].Strength != tt.wantStrength {
				t.Errorf("Strength = %q, want %q", got[0].Strength, tt.wantStrength)
			}
			if tt.wantScore != 0 && got[0].Score != tt.wantScore {
				t.Errorf("Score = %f, want %f", got[0].Score, tt.wantScore)
			}
		})
	}
}

func TestMatchScored_SortedByScore(t *testing.T) {
	s := NewStore()
	// skill-a: 2 tokens, both matched by command → score 1.0
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "skill-a", Description: "sort group", AutoTrigger: true}})
	// skill-b: 4 tokens, 2 matched → score 0.5
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "skill-b", Description: "sort group organize files", AutoTrigger: true}})

	got := s.MatchScored("sort group")
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Score < got[1].Score {
		t.Errorf("results not sorted descending: [0].Score=%f [1].Score=%f", got[0].Score, got[1].Score)
	}
	if got[0].Skill.Metadata.Name != "skill-a" {
		t.Errorf("expected skill-a first (score 1.0), got %q", got[0].Skill.Metadata.Name)
	}
}

func TestMatchScored_StableSortTieBreaker(t *testing.T) {
	s := NewStore()
	// Both skills have 2 tokens; command matches both → score 1.0 each.
	// Alphabetical tie-breaker should place "alpha-skill" before "beta-skill".
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "beta-skill", Description: "sort group", AutoTrigger: true}})
	s.Upsert(&Skill{Metadata: SkillMetadata{Name: "alpha-skill", Description: "sort group", AutoTrigger: true}})

	got := s.MatchScored("sort group")
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Score != got[1].Score {
		t.Fatalf("expected equal scores for tie-breaker test, got %f and %f", got[0].Score, got[1].Score)
	}
	if got[0].Skill.Metadata.Name != "alpha-skill" {
		t.Errorf("expected alpha-skill first (alphabetical tie-breaker), got %q", got[0].Skill.Metadata.Name)
	}
}

func TestSkillTokens_WhitespaceTags(t *testing.T) {
	sk := Skill{Metadata: SkillMetadata{
		Description: "organize files",
		Tags:        []string{" sort ", "  ", "tidy"},
	}}
	got := skillTokens(sk)
	gotSet := make(map[string]bool, len(got))
	for _, tok := range got {
		gotSet[tok] = true
	}
	if !gotSet["sort"] {
		t.Errorf("expected trimmed tag %q in tokens %v", "sort", got)
	}
	if !gotSet["tidy"] {
		t.Errorf("expected tag %q in tokens %v", "tidy", got)
	}
	for _, tok := range got {
		if tok == "" || tok == "  " || tok == " " {
			t.Errorf("blank/whitespace tag should not appear in tokens, got %q", tok)
		}
	}
}

func TestMatchHybrid(t *testing.T) {
	// strongSkill returns a skill whose 4-token description is matched strongly
	// by the command "sort group my docs" (matches "sort" and "group" → 2/4 = 0.5, strong).
	strongSkill := func(name string) Skill {
		return Skill{Metadata: SkillMetadata{
			Name:        name,
			Description: "sort group organize files",
			AutoTrigger: true,
		}}
	}

	tests := []struct {
		name       string
		skills     []Skill
		command    string
		mock       *mockDisambiguator
		wantLen    int
		wantCalled bool
	}{
		{
			name:       "single strong match no LLM call",
			skills:     []Skill{strongSkill("organizer")},
			command:    "sort group my docs",
			mock:       &mockDisambiguator{},
			wantLen:    1,
			wantCalled: false,
		},
		{
			name:    "multiple strong matches calls LLM",
			skills:  []Skill{strongSkill("organizer"), strongSkill("sorter")},
			command: "sort group my docs",
			mock: &mockDisambiguator{
				result: []Skill{strongSkill("organizer")},
			},
			wantLen:    1,
			wantCalled: true,
		},
		{
			name:       "nil LLM graceful degradation",
			skills:     []Skill{strongSkill("organizer"), strongSkill("sorter")},
			command:    "sort group my docs",
			mock:       nil,
			wantLen:    2,
			wantCalled: false,
		},
		{
			name:       "no matches LLM not called",
			skills:     []Skill{strongSkill("organizer")},
			command:    "delete everything",
			mock:       &mockDisambiguator{},
			wantLen:    0,
			wantCalled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i := range tt.skills {
				s.Upsert(&tt.skills[i])
			}
			var llm LLMDisambiguator
			if tt.mock != nil {
				llm = tt.mock
			}
			got := s.MatchHybrid(tt.command, llm)
			if len(got) != tt.wantLen {
				t.Errorf("len(results) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.mock != nil && tt.mock.called != tt.wantCalled {
				t.Errorf("LLM called = %v, want %v", tt.mock.called, tt.wantCalled)
			}
		})
	}
}
