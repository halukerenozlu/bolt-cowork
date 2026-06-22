package views

import (
	"strings"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	const url = "https://example.com/docs"
	tests := []struct {
		name        string
		goos        string
		wantCommand string
		wantArgs    []string
	}{
		{
			name:        "Windows",
			goos:        "windows",
			wantCommand: "rundll32",
			wantArgs:    []string{"url.dll,FileProtocolHandler", url},
		},
		{
			name:        "macOS",
			goos:        "darwin",
			wantCommand: "open",
			wantArgs:    []string{url},
		},
		{
			name:        "Linux",
			goos:        "linux",
			wantCommand: "xdg-open",
			wantArgs:    []string{url},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := browserCommand(tt.goos, url)
			if !strings.HasSuffix(cmd.Path, tt.wantCommand) && !strings.HasSuffix(cmd.Path, tt.wantCommand+".exe") {
				t.Fatalf("command path = %q, want suffix %q", cmd.Path, tt.wantCommand)
			}
			if got := cmd.Args[1:]; strings.Join(got, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Fatalf("args = %v, want %v", got, tt.wantArgs)
			}
		})
	}
}
