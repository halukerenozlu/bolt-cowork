package skill

import (
	"errors"
	"testing"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore() returned nil")
	}
	skills := s.GetAll()
	if len(skills) != 0 {
		t.Errorf("new store GetAll() len = %d, want 0", len(skills))
	}
}

func TestGetByName_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetByName("nonexistent")
	if !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("GetByName missing skill: got err %v, want ErrSkillNotFound", err)
	}
}

func TestGetByName_Found(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "test-skill", Description: "desc", Content: "body"})

	got, err := s.GetByName("test-skill")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", got.Name, "test-skill")
	}
	if got.Description != "desc" {
		t.Errorf("Description = %q, want %q", got.Description, "desc")
	}
}

func TestGetAll_ReturnsCopy(t *testing.T) {
	s := NewStore()
	s.Upsert(&Skill{Name: "a"})
	s.Upsert(&Skill{Name: "b"})

	first := s.GetAll()
	if len(first) != 2 {
		t.Fatalf("GetAll len = %d, want 2", len(first))
	}

	// Mutating the returned slice must not affect the store.
	first[0].Name = "mutated"
	second := s.GetAll()
	if second[0].Name == "mutated" {
		t.Error("GetAll should return a copy, but returned a reference")
	}
}
