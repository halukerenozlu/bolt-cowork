package skill

import (
	"testing"
)

func TestSearchByTag(t *testing.T) {
	tests := []struct {
		name    string
		skills  []Skill
		query   string
		wantLen int
	}{
		{
			name: "exact match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Tags: []string{"files", "sort"}}},
			},
			query:   "files",
			wantLen: 1,
		},
		{
			name: "case-insensitive match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Tags: []string{"Files"}}},
			},
			query:   "files",
			wantLen: 1,
		},
		{
			name: "no match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Tags: []string{"files"}}},
			},
			query:   "images",
			wantLen: 0,
		},
		{
			name:    "empty store",
			skills:  nil,
			query:   "files",
			wantLen: 0,
		},
		{
			name: "trimmed whitespace input matches",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Tags: []string{"files"}}},
			},
			query:   " files ",
			wantLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i := range tt.skills {
				s.Upsert(&tt.skills[i])
			}
			got := s.SearchByTag(tt.query)
			if len(got) != tt.wantLen {
				t.Errorf("SearchByTag(%q) = %d results, want %d", tt.query, len(got), tt.wantLen)
			}
		})
	}
}

func TestListCategories(t *testing.T) {
	tests := []struct {
		name     string
		skills   []Skill
		wantCats []string
		wantLen  int
	}{
		{
			name: "multiple categories sorted",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Category: "filesystem"}},
				{Metadata: SkillMetadata{Name: "b", Category: "content"}},
			},
			wantCats: []string{"content", "filesystem"},
			wantLen:  2,
		},
		{
			name: "duplicates removed",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Category: "filesystem"}},
				{Metadata: SkillMetadata{Name: "b", Category: "filesystem"}},
			},
			wantCats: []string{"filesystem"},
			wantLen:  1,
		},
		{
			name: "empty category skipped",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Category: ""}},
				{Metadata: SkillMetadata{Name: "b", Category: "tools"}},
			},
			wantCats: []string{"tools"},
			wantLen:  1,
		},
		{
			name:     "empty store",
			skills:   nil,
			wantCats: []string{},
			wantLen:  0,
		},
		{
			name: "case-insensitive dedup first-seen wins",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "a", Category: "Filesystem"}},
				{Metadata: SkillMetadata{Name: "b", Category: "filesystem"}},
			},
			wantCats: []string{"Filesystem"},
			wantLen:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i := range tt.skills {
				s.Upsert(&tt.skills[i])
			}
			got := s.ListCategories()
			if len(got) != tt.wantLen {
				t.Fatalf("ListCategories() = %v, want %v", got, tt.wantCats)
			}
			for i, cat := range tt.wantCats {
				if got[i] != cat {
					t.Errorf("ListCategories()[%d] = %q, want %q", i, got[i], cat)
				}
			}
		})
	}
}

func TestGetByCategory(t *testing.T) {
	tests := []struct {
		name     string
		skills   []Skill
		query    string
		wantLen  int
		wantName string
	}{
		{
			name: "exact match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "organizer", Category: "filesystem"}},
			},
			query:    "filesystem",
			wantLen:  1,
			wantName: "organizer",
		},
		{
			name: "case-insensitive match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "organizer", Category: "Filesystem"}},
			},
			query:    "filesystem",
			wantLen:  1,
			wantName: "organizer",
		},
		{
			name: "no match",
			skills: []Skill{
				{Metadata: SkillMetadata{Name: "organizer", Category: "filesystem"}},
			},
			query:   "content",
			wantLen: 0,
		},
		{
			name:    "empty store",
			skills:  nil,
			query:   "filesystem",
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i := range tt.skills {
				s.Upsert(&tt.skills[i])
			}
			got := s.GetByCategory(tt.query)
			if len(got) != tt.wantLen {
				t.Fatalf("GetByCategory(%q) = %d results, want %d", tt.query, len(got), tt.wantLen)
			}
			if tt.wantName != "" && got[0].Metadata.Name != tt.wantName {
				t.Errorf("got[0].Name = %q, want %q", got[0].Metadata.Name, tt.wantName)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	base := []Skill{
		{Metadata: SkillMetadata{
			Name:        "file-organizer",
			Description: "sorts files by extension into directories",
			Tags:        []string{"tidy", "sort"},
			Category:    "filesystem",
		}},
		{Metadata: SkillMetadata{
			Name:        "summarizer",
			Description: "produces a brief summary of documents",
			Tags:        []string{"recap"},
			Category:    "content",
		}},
	}

	tests := []struct {
		name    string
		query   string
		wantLen int
	}{
		{"matches name", "organizer", 1},
		{"matches description", "extension", 1},
		{"matches tag", "tidy", 1},
		{"matches category", "content", 1},
		{"no match", "zzz", 0},
		{"empty query returns all", "", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i := range base {
				s.Upsert(&base[i])
			}
			got := s.Search(tt.query)
			if len(got) != tt.wantLen {
				t.Errorf("Search(%q) = %d results, want %d", tt.query, len(got), tt.wantLen)
			}
		})
	}
}
