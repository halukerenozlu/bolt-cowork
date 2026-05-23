package views

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWelcomeLogo(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		want      []string
		notWant   []string
		wantLines int
	}{
		{
			name:      "compact",
			width:     60,
			want:      []string{"BOLT", "Cowork"},
			notWant:   []string{"██████"},
			wantLines: 1,
		},
		{
			name:      "wide",
			width:     90,
			want:      []string{"██████╗", "C o w o r k", "╚═════╝"},
			notWant:   []string{"BOLT ⚡ Cowork", "⚡"},
			wantLines: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lipgloss.NewStyle().Render(welcomeLogo(tt.width))
			plain := stripANSI(got)

			for _, want := range tt.want {
				if !strings.Contains(plain, want) {
					t.Fatalf("welcomeLogo(%d) missing %q:\n%s", tt.width, want, plain)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(plain, notWant) {
					t.Fatalf("welcomeLogo(%d) unexpectedly contains %q:\n%s", tt.width, notWant, plain)
				}
			}
			if lines := strings.Count(plain, "\n") + 1; lines != tt.wantLines {
				t.Fatalf("welcomeLogo(%d) lines = %d, want %d:\n%s", tt.width, lines, tt.wantLines, plain)
			}
		})
	}
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
