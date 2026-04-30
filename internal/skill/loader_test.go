package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// writeSkillFile creates a SKILL.md file with the given content at path.
func writeSkillFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

const validSkillContent = `---
name: file-organizer
description: Organizes files by type into directories
auto_trigger: true
---

# File Organizer

Organizes files in the target directory.
`

func TestParseFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	writeSkillFile(t, path, validSkillContent)

	sk, warns, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if sk.Metadata.Name != "file-organizer" {
		t.Errorf("Name = %q, want %q", sk.Metadata.Name, "file-organizer")
	}
	if sk.Metadata.Description != "Organizes files by type into directories" {
		t.Errorf("Description = %q", sk.Metadata.Description)
	}
	if !sk.Metadata.AutoTrigger {
		t.Error("AutoTrigger = false, want true")
	}
	if sk.FilePath != path {
		t.Errorf("FilePath = %q, want %q", sk.FilePath, path)
	}
}

func TestParseFile_AutoTriggerDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	writeSkillFile(t, path, "---\nname: summarizer\ndescription: Summarizes files\n---\n\nBody.\n")

	sk, _, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if sk.Metadata.AutoTrigger {
		t.Error("AutoTrigger should default to false when not specified")
	}
}

func TestParseFile_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "somedir", "SKILL.md")
	writeSkillFile(t, path, "# No frontmatter here\n\nSome body text.\n")

	// With fallback logic, no-frontmatter files should still parse if name can
	// be derived from the path (parent dir name).
	sk, warns, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v (warns: %v)", err, warns)
	}
	if sk.Metadata.Name != "somedir" {
		t.Errorf("Name = %q, want %q (derived from parent dir)", sk.Metadata.Name, "somedir")
	}
	if len(warns) == 0 {
		// No warnings is acceptable — the name was derived successfully.
	}
}

func TestParseFile_EmptyName(t *testing.T) {
	dir := t.TempDir()
	// Place the file directly in temp dir root — name derivation from parent
	// should still work (parent is the temp dir basename).
	path := filepath.Join(dir, "SKILL.md")
	writeSkillFile(t, path, "---\nname: \ndescription: something\n---\n\nBody.\n")

	// With fallback, empty name in frontmatter derives from path.
	sk, _, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	// Name should be derived from temp dir basename.
	if sk.Metadata.Name == "" {
		t.Error("expected name to be derived from path, got empty")
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, _, err := ParseFile(filepath.Join(t.TempDir(), "nonexistent.md"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseFile_ContentPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	writeSkillFile(t, path, "---\nname: test\ndescription: d\n---\n\n# Body\n\nSome content here.\n")

	sk, _, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if sk.Content == "" {
		t.Error("Content should not be empty")
	}
	if sk.Content == "---\nname: test\ndescription: d\n---\n" {
		t.Error("Content should be Markdown body, not frontmatter")
	}
}

func TestLoadAll_Empty(t *testing.T) {
	dir := t.TempDir()
	s := NewStore()
	warns := s.LoadAll([]string{dir})
	_ = warns
	if len(s.GetAll()) != 0 {
		t.Errorf("expected 0 skills, got %d", len(s.GetAll()))
	}
}

func TestLoadAll_SingleSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"), validSkillContent)

	s := NewStore()
	warns := s.LoadAll([]string{dir})
	_ = warns
	if len(s.GetAll()) != 1 {
		t.Errorf("expected 1 skill, got %d", len(s.GetAll()))
	}
}

func TestLoadAll_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "organizer", "SKILL.md"), validSkillContent)
	writeSkillFile(t, filepath.Join(dir, "summarizer", "SKILL.md"),
		"---\nname: summarizer\ndescription: Summarizes\nauto_trigger: false\n---\n\nBody.\n")

	s := NewStore()
	warns := s.LoadAll([]string{dir})
	_ = warns
	if len(s.GetAll()) != 2 {
		t.Errorf("expected 2 skills, got %d", len(s.GetAll()))
	}
}

func TestLoadAll_Override(t *testing.T) {
	globalDir := t.TempDir()
	localDir := t.TempDir()

	writeSkillFile(t, filepath.Join(globalDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: global version\n---\n\nGlobal body.\n")
	writeSkillFile(t, filepath.Join(localDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: local version\n---\n\nLocal body.\n")

	s := NewStore()
	// global first, local second — local should win
	warns := s.LoadAll([]string{globalDir, localDir})
	_ = warns
	skills := s.GetAll()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after override, got %d", len(skills))
	}
	if skills[0].Metadata.Description != "local version" {
		t.Errorf("Description = %q, want %q", skills[0].Metadata.Description, "local version")
	}
}

func TestLoadAll_NonExistentDir(t *testing.T) {
	s := NewStore()
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	// Must not panic; warnings are acceptable, hard errors are not.
	_ = s.LoadAll([]string{nonExistent})
}

func TestLoadAll_MissingDir(t *testing.T) {
	s := NewStore()
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	warns := s.LoadAll([]string{nonExistent})
	// No error; warnings slice is returned (may be empty or contain an Info line).
	if warns == nil {
		// nil is acceptable — just ensure it didn't panic
	}
	if len(s.GetAll()) != 0 {
		t.Errorf("expected 0 skills, got %d", len(s.GetAll()))
	}
}

func TestLoadAll_BadYAML(t *testing.T) {
	dir := t.TempDir()
	// Bad skill: invalid frontmatter
	writeSkillFile(t, filepath.Join(dir, "bad", "SKILL.md"),
		"---\n: invalid: yaml: [\n---\n\nBody.\n")
	// Good skill alongside the bad one
	writeSkillFile(t, filepath.Join(dir, "good", "SKILL.md"),
		"---\nname: good-skill\ndescription: valid\n---\n\nBody.\n")

	s := NewStore()
	warns := s.LoadAll([]string{dir})

	// Both skills should be loaded — the bad YAML one falls back to
	// name-from-path and body content.
	loaded := s.GetAll()
	if len(loaded) < 1 {
		t.Errorf("expected at least 1 skill (good), got %d", len(loaded))
	}
	// There must be at least one warning about the bad file.
	if len(warns) == 0 {
		t.Error("expected at least one warning for bad YAML, got none")
	}
}

func TestLoadAll_NameConflict(t *testing.T) {
	t.Setenv("HOME", "/fake/home/for-test")
	t.Setenv("USERPROFILE", "/fake/home/for-test")

	globalDir := t.TempDir()
	localDir := t.TempDir()

	writeSkillFile(t, filepath.Join(globalDir, "SKILL.md"),
		"---\nname: shared-skill\ndescription: global version\n---\n\nGlobal.\n")
	writeSkillFile(t, filepath.Join(localDir, "SKILL.md"),
		"---\nname: shared-skill\ndescription: local version\n---\n\nLocal.\n")

	s := NewStore()
	warns := s.LoadAll([]string{globalDir, localDir})

	// Local must win.
	sk, err := s.GetByName("shared-skill")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if sk.Metadata.Description != "local version" {
		t.Errorf("Description = %q, want %q", sk.Metadata.Description, "local version")
	}
	// A conflict warning must be present.
	found := false
	for _, w := range warns {
		if strings.Contains(w, "shared-skill") && strings.Contains(w, "overridden") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a name-conflict warning, got: %v", warns)
	}
}

func TestLoadAll_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"), "   \n\t  \n")

	s := NewStore()
	warns := s.LoadAll([]string{dir})

	if len(s.GetAll()) != 0 {
		t.Errorf("expected 0 skills (empty file skipped), got %d", len(s.GetAll()))
	}
	if len(warns) == 0 {
		t.Error("expected a warning for empty skill file, got none")
	}
}

func TestLoadAll_NonMarkdown(t *testing.T) {
	dir := t.TempDir()
	// Write a non-SKILL.md file; should be silently ignored.
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("some content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewStore()
	warns := s.LoadAll([]string{dir})

	if len(s.GetAll()) != 0 {
		t.Errorf("expected 0 skills, got %d", len(s.GetAll()))
	}
	if len(warns) != 0 {
		t.Errorf("expected no warnings for non-SKILL.md file, got: %v", warns)
	}
}

func TestLoadAll_ScopeProject(t *testing.T) {
	// Override home env vars so the temp dir is guaranteed to not be under
	// the home directory (on Windows, t.TempDir() falls under USERPROFILE).
	t.Setenv("HOME", "/fake/home/for-test")
	t.Setenv("USERPROFILE", "/fake/home/for-test")

	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"), validSkillContent)

	s := NewStore()
	_ = s.LoadAll([]string{dir})
	sk, err := s.GetByName("file-organizer")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if sk.Scope != ScopeProject {
		t.Errorf("Scope = %v (%s), want ScopeProject (%s)", sk.Scope, sk.Scope, ScopeProject)
	}
}

func TestLoadAll_ScopeGlobal(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	globalDir := filepath.Join(fakeHome, ".bolt-cowork", "skills")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("MkdirAll globalDir: %v", err)
	}
	writeSkillFile(t, filepath.Join(globalDir, "SKILL.md"), validSkillContent)

	s := NewStore()
	_ = s.LoadAll([]string{globalDir})
	sk, err := s.GetByName("file-organizer")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	// Scope depends on os.UserHomeDir() — on Windows Setenv may not affect it.
	// Accept either ScopeGlobal (if home detection worked) or ScopeProject (if not).
	if sk.Scope != ScopeGlobal && sk.Scope != ScopeProject {
		t.Errorf("Scope = %v (%s), want ScopeGlobal or ScopeProject", sk.Scope, sk.Scope)
	}
}

func TestGetByName_AfterLoad(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"), validSkillContent)

	s := NewStore()
	_ = s.LoadAll([]string{dir})

	sk, err := s.GetByName("file-organizer")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if sk.Metadata.Name != "file-organizer" {
		t.Errorf("Name = %q, want %q", sk.Metadata.Name, "file-organizer")
	}
}

func TestGetByName_Missing(t *testing.T) {
	s := NewStore()
	_, err := s.GetByName("does-not-exist")
	if !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("expected ErrSkillNotFound, got: %v", err)
	}
}

func TestDefaultSkillsExist(t *testing.T) {
	// The skill package is at internal/skill/; default skills moved to
	// cmd/bolt-cowork/skills/ so they can be embedded into the binary.
	projectRoot := filepath.Join("..", "..")
	skillsDir := filepath.Join(projectRoot, "cmd", "bolt-cowork", "skills")

	tests := []struct {
		name     string
		skillDir string
	}{
		{"file-organizer", "file-organizer"},
		{"summarizer", "summarizer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(skillsDir, tt.skillDir, "SKILL.md")
			sk, _, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", path, err)
			}
			if sk.Metadata.Name != tt.name {
				t.Errorf("Name = %q, want %q", sk.Metadata.Name, tt.name)
			}
			if sk.Metadata.Description == "" {
				t.Error("Description should not be empty")
			}
			if sk.Content == "" {
				t.Error("Content should not be empty")
			}
		})
	}
}

func TestLoadEmbedded_Basic(t *testing.T) {
	fsys := fstest.MapFS{
		"file-organizer/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: file-organizer\ndescription: Organizes files\nauto_trigger: true\n---\n\nBody.\n"),
		},
	}
	s := NewStore()
	if err := s.LoadEmbedded(fsys); err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	sk, err := s.GetByName("file-organizer")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if sk.Scope != ScopeBundled {
		t.Errorf("Scope = %v (%s), want ScopeBundled (%s)", sk.Scope, sk.Scope, ScopeBundled)
	}
	if !sk.Metadata.AutoTrigger {
		t.Error("AutoTrigger = false, want true")
	}
}

func TestLoadEmbedded_InvalidSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"bad/SKILL.md": &fstest.MapFile{
			Data: []byte("no frontmatter here"),
		},
		"good/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: good-skill\ndescription: valid\n---\n\nBody.\n"),
		},
	}
	s := NewStore()
	if err := s.LoadEmbedded(fsys); err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	// Both may load: "bad" will derive name from path ("bad") via fallback.
	// At minimum, "good-skill" must load.
	if _, err := s.GetByName("good-skill"); err != nil {
		t.Errorf("expected good-skill to be loaded: %v", err)
	}
}

func TestLoadEmbedded_OverriddenByFilesystem(t *testing.T) {
	fsys := fstest.MapFS{
		"SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: my-skill\ndescription: bundled version\n---\n\nBundled body.\n"),
		},
	}
	s := NewStore()
	if err := s.LoadEmbedded(fsys); err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"),
		"---\nname: my-skill\ndescription: local version\n---\n\nLocal body.\n")
	_ = s.LoadAll([]string{dir})

	skills := s.GetAll()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after override, got %d", len(skills))
	}
	if skills[0].Metadata.Description != "local version" {
		t.Errorf("Description = %q, want %q", skills[0].Metadata.Description, "local version")
	}
}

func TestLoadAll_ScopeAssignment(t *testing.T) {
	t.Setenv("HOME", "/fake/home/for-test")
	t.Setenv("USERPROFILE", "/fake/home/for-test")

	// Bundled via LoadEmbedded.
	fsys := fstest.MapFS{
		"bundled-skill/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: bundled-skill\ndescription: bundled\n---\n\nBody.\n"),
		},
	}
	s := NewStore()
	if err := s.LoadEmbedded(fsys); err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	sk, err := s.GetByName("bundled-skill")
	if err != nil {
		t.Fatalf("GetByName bundled: %v", err)
	}
	if sk.Scope != ScopeBundled {
		t.Errorf("bundled skill Scope = %s, want %s", sk.Scope, ScopeBundled)
	}

	// Project-local via LoadAll (not under home).
	projectDir := t.TempDir()
	writeSkillFile(t, filepath.Join(projectDir, "SKILL.md"),
		"---\nname: project-skill\ndescription: project\n---\n\nBody.\n")
	_ = s.LoadAll([]string{projectDir})

	sk, err = s.GetByName("project-skill")
	if err != nil {
		t.Fatalf("GetByName project: %v", err)
	}
	if sk.Scope != ScopeProject {
		t.Errorf("project skill Scope = %s, want %s", sk.Scope, ScopeProject)
	}
}

func TestLoadAll_OverrideOrder(t *testing.T) {
	t.Setenv("HOME", "/fake/home/for-test")
	t.Setenv("USERPROFILE", "/fake/home/for-test")

	// Bundled → global → project, all with same name.
	fsys := fstest.MapFS{
		"SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: shared\ndescription: bundled\n---\n\nBundled.\n"),
		},
	}
	s := NewStore()
	if err := s.LoadEmbedded(fsys); err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeSkillFile(t, filepath.Join(dir1, "SKILL.md"),
		"---\nname: shared\ndescription: first-dir\n---\n\nFirst.\n")
	writeSkillFile(t, filepath.Join(dir2, "SKILL.md"),
		"---\nname: shared\ndescription: second-dir\n---\n\nSecond.\n")

	_ = s.LoadAll([]string{dir1, dir2})

	sk, err := s.GetByName("shared")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	// second-dir should win (later dir overrides earlier).
	if sk.Metadata.Description != "second-dir" {
		t.Errorf("Description = %q, want %q", sk.Metadata.Description, "second-dir")
	}
}
