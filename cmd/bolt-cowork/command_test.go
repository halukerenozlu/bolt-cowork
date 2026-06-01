package main

import (
	"os"
	"sort"
	"strings"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewCommandRegistry()
	cmd := &SlashCommand{
		Name:        "/test",
		Description: "A test command",
		Usage:       "/test",
		Category:    "General",
		Execute: func(args []string, ctx *CommandContext) error {
			return nil
		},
	}
	r.Register(cmd)

	got, ok := r.Get("/test")
	if !ok {
		t.Fatal("Get returned false for registered command")
	}
	if got.Name != "/test" {
		t.Errorf("Name = %q, want %q", got.Name, "/test")
	}
	if got.Description != "A test command" {
		t.Errorf("Description = %q, want %q", got.Description, "A test command")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewCommandRegistry()
	_, ok := r.Get("/nonexistent")
	if ok {
		t.Error("Get returned true for unregistered command")
	}
}

func TestRegistryNames(t *testing.T) {
	r := NewCommandRegistry()
	names := []string{"/zebra", "/alpha", "/mid"}
	for _, n := range names {
		r.Register(&SlashCommand{
			Name:     n,
			Category: "General",
			Execute:  func(args []string, ctx *CommandContext) error { return nil },
		})
	}

	got := r.Names()
	want := make([]string, len(names))
	copy(want, names)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("Names() returned %d items, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewCommandRegistry()
	names := []string{"/zebra", "/alpha", "/mid"}
	for _, n := range names {
		r.Register(&SlashCommand{
			Name:     n,
			Category: "General",
			Execute:  func(args []string, ctx *CommandContext) error { return nil },
		})
	}

	got := r.All()
	want := []string{"/alpha", "/mid", "/zebra"}

	if len(got) != len(want) {
		t.Fatalf("All() returned %d items, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("All()[%d].Name = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestRegistryByCategory(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&SlashCommand{Name: "/a", Category: "General", Execute: func(args []string, ctx *CommandContext) error { return nil }})
	r.Register(&SlashCommand{Name: "/b", Category: "General", Execute: func(args []string, ctx *CommandContext) error { return nil }})
	r.Register(&SlashCommand{Name: "/c", Category: "Config", Execute: func(args []string, ctx *CommandContext) error { return nil }})

	cats := r.ByCategory()

	if len(cats["General"]) != 2 {
		t.Errorf("General category has %d commands, want 2", len(cats["General"]))
	}
	if len(cats["Config"]) != 1 {
		t.Errorf("Config category has %d commands, want 1", len(cats["Config"]))
	}

	// Check sorting within category.
	if cats["General"][0].Name != "/a" || cats["General"][1].Name != "/b" {
		t.Errorf("General commands not sorted: got %v, %v", cats["General"][0].Name, cats["General"][1].Name)
	}
}

func TestHelpAutoGenerate(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&SlashCommand{
		Name:        "/visible",
		Description: "A visible command",
		Usage:       "/visible",
		Category:    "General",
		Execute:     func(args []string, ctx *CommandContext) error { return nil },
	})
	r.Register(&SlashCommand{
		Name:        "/hidden",
		Description: "A hidden command",
		Usage:       "/hidden",
		Category:    "General",
		Hidden:      true,
		Execute:     func(args []string, ctx *CommandContext) error { return nil },
	})
	r.Register(&SlashCommand{
		Name:        "/config-cmd",
		Description: "Config command",
		Usage:       "/config-cmd [opts]",
		Category:    "Config",
		Execute:     func(args []string, ctx *CommandContext) error { return nil },
	})

	output := captureStderr(func() {
		printAutoHelp(r)
	})

	// Non-hidden commands should appear.
	if !strings.Contains(output, "/visible") {
		t.Errorf("help output missing /visible:\n%s", output)
	}
	if !strings.Contains(output, "A visible command") {
		t.Errorf("help output missing description for /visible:\n%s", output)
	}
	if !strings.Contains(output, "/config-cmd") {
		t.Errorf("help output missing /config-cmd:\n%s", output)
	}

	// Hidden commands should NOT appear.
	if strings.Contains(output, "/hidden") {
		t.Errorf("help output should not contain hidden command /hidden:\n%s", output)
	}

	// Category headers should appear.
	if !strings.Contains(output, "General:") {
		t.Errorf("help output missing 'General:' header:\n%s", output)
	}
	if !strings.Contains(output, "Config:") {
		t.Errorf("help output missing 'Config:' header:\n%s", output)
	}
}

func TestDefaultRegistryContainsAllCommands(t *testing.T) {
	r := NewCommandRegistry()
	RegisterDefaultCommands(r)

	expected := []string{"/help", "/clear", "/quit", "/config", "/mode",
		"/skills", "/skill", "/use", "/model", "/key", "/dir", "/init"}

	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("default registry missing command %q", name)
		}
	}
}

func TestHandleSlashCommandViaRegistry(t *testing.T) {
	r := NewCommandRegistry()
	RegisterDefaultCommands(r)

	// /quit should return true (exit).
	ctx := &CommandContext{
		Cfg:         nil,
		History:     nil,
		Store:       nil,
		ForceSkills: nil,
		PreviousDir: nil,
		LineReader:  nil,
	}

	old := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	exit := handleSlashCommand("/quit", r, ctx)

	w.Close()
	os.Stderr = old

	if !exit {
		t.Error("handleSlashCommand(/quit) should return true")
	}
}
