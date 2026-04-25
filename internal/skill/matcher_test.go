package skill

import (
	"testing"
)

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
	s.Upsert(&Skill{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true})

	got := s.Match("organize files")
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Name != "file-organizer" {
		t.Errorf("expected file-organizer, got %s", got[0].Name)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "file-organizer", Description: "Organizes files", AutoTrigger: true})

	got := s.Match("delete all")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(got))
	}
}

func TestMatch_MultipleSkills(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true})
	s.Upsert(&Skill{Name: "file-cleaner", Description: "Cleans old files", AutoTrigger: true})

	got := s.Match("clean and organize files")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "file-organizer", Description: "Organizes files by type", AutoTrigger: true})

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
	s.Upsert(&Skill{Name: "manual-skill", Description: "Organizes files", AutoTrigger: false})

	got := s.Match("organize files")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for AutoTrigger=false, got %d", len(got))
	}
}

func TestMatch_StopWordsIgnored(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "stop-only", Description: "the a an is are", AutoTrigger: true})

	got := s.Match("the a an is are")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for stop-words-only description, got %d", len(got))
	}
}

func TestMatch_EmptyCommand(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "file-organizer", Description: "Organizes files", AutoTrigger: true})

	got := s.Match("")
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for empty command, got %d", len(got))
	}
}
