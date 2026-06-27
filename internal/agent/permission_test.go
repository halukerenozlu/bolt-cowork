package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

func TestPermissionReason_FileDelete(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	reason := dangerReason(Step{Action: ActionDelete, Path: filepath.Join(dir, "file.txt")}, sb)
	if !strings.Contains(reason, "permanently removes") {
		t.Errorf("dangerReason(delete) = %q, want it to contain %q", reason, "permanently removes")
	}
}

func TestPermissionReason_FileOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("data"), 0644)

	sb, _ := sandbox.New(dir)

	reason := dangerReason(Step{Action: ActionWrite, Path: existing}, sb)
	if !strings.Contains(reason, "overwrites existing") {
		t.Errorf("dangerReason(write existing) = %q, want it to contain %q", reason, "overwrites existing")
	}
}

func TestPermissionReason_OutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	outside := filepath.Join(filepath.Dir(dir), "outside.txt")
	reason := dangerReason(Step{Action: ActionWrite, Path: outside}, sb)
	if reason != "writes to file" {
		t.Errorf("dangerReason(write outside) = %q, want %q", reason, "writes to file")
	}
}

func TestPermissionReason_EmptyForSafe(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "file.txt")
	os.WriteFile(existing, []byte("x"), 0644)

	sb, _ := sandbox.New(dir)

	safeActions := []StepAction{ActionRead, ActionList}
	for _, action := range safeActions {
		t.Run(string(action), func(t *testing.T) {
			reason := dangerReason(Step{Action: action, Path: existing}, sb)
			if reason != "" {
				t.Errorf("dangerReason(%s) = %q, want empty", action, reason)
			}
			if isDangerous(Step{Action: action, Path: existing}) {
				t.Errorf("isDangerous(%s) = true, want false", action)
			}
		})
	}
}

func TestPermissionReason_ReasonFormat(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "file.txt")
	os.WriteFile(existing, []byte("x"), 0644)

	sb, _ := sandbox.New(dir)

	dangerousSteps := []struct {
		name string
		step Step
	}{
		{"delete", Step{Action: ActionDelete, Path: existing}},
		{"write existing", Step{Action: ActionWrite, Path: existing}},
		{"write new", Step{Action: ActionWrite, Path: filepath.Join(dir, "new.txt")}},
		{"move", Step{Action: ActionMove, Path: existing, Destination: filepath.Join(dir, "moved.txt")}},
		{"rename", Step{Action: ActionRename, Path: existing, Destination: filepath.Join(dir, "renamed.txt")}},
		{"copy", Step{Action: ActionCopy, Path: existing, Destination: filepath.Join(dir, "copied.txt")}},
		{"mkdir", Step{Action: ActionMkdir, Path: filepath.Join(dir, "newdir")}},
	}

	for _, tt := range dangerousSteps {
		t.Run(tt.name, func(t *testing.T) {
			reason := dangerReason(tt.step, sb)
			if reason == "" {
				t.Fatal("dangerReason returned empty for dangerous action")
			}
			if strings.TrimRight(reason, " \t\n") != reason {
				t.Errorf("dangerReason has trailing whitespace: %q", reason)
			}
			if unicode.IsUpper(rune(reason[0])) {
				t.Errorf("dangerReason starts with uppercase: %q", reason)
			}
		})
	}
}
