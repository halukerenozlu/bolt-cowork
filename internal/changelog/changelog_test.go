package changelog

import (
	"strings"
	"testing"
	"time"
)

const sample = `# Changelog

## [Unreleased]

### Added

- new thing

## [v0.4.5] - 2026-06-27

### Fixed

- old thing

## [0.3.5] - 2026-05-19

### Added

- ancient thing

[v0.4.5]: https://example.test/compare/v0.4.4...v0.4.5
[0.3.5]: https://example.test/compare/v0.3.4...v0.3.5
`

// emptyUnreleased has the heading but no entries under it.
const emptyUnreleased = `# Changelog

## [Unreleased]

## [v0.4.5] - 2026-06-27

### Fixed

- old thing
`

var releaseDate = time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)

func TestExtract(t *testing.T) {
	tests := []struct {
		name    string
		content string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "v-prefixed heading",
			content: sample,
			version: "v0.4.5",
			want:    "### Fixed\n\n- old thing",
		},
		{
			name:    "heading written without v prefix",
			content: sample,
			version: "v0.3.5",
			want:    "### Added\n\n- ancient thing",
		},
		{
			name:    "caller omits v prefix",
			content: sample,
			version: "0.4.5",
			want:    "### Fixed\n\n- old thing",
		},
		{
			name:    "last section runs to end of file",
			content: "# Changelog\n\n## [v0.1.0] - 2026-01-01\n\n- first\n",
			version: "v0.1.0",
			want:    "- first",
		},
		{
			name:    "crlf input yields clean body",
			content: strings.ReplaceAll(sample, "\n", "\r\n"),
			version: "v0.4.5",
			want:    "### Fixed\n\n- old thing",
		},
		{
			name:    "unknown version",
			content: sample,
			version: "v9.9.9",
			wantErr: true,
		},
		{
			name:    "empty section",
			content: emptyUnreleased,
			version: "Unreleased",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Extract(tt.content, tt.version)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Extract(%q) = %q, want error", tt.version, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract(%q) returned error: %v", tt.version, err)
			}
			if got != tt.want {
				t.Errorf("Extract(%q) =\n%q\nwant\n%q", tt.version, got, tt.want)
			}
		})
	}
}

func TestCut(t *testing.T) {
	got, err := Cut(sample, "v0.4.6", releaseDate, "https://example.test")
	if err != nil {
		t.Fatalf("Cut returned error: %v", err)
	}

	t.Run("keeps an empty Unreleased section on top", func(t *testing.T) {
		body, err := Extract(got, "v0.4.6")
		if err != nil {
			t.Fatalf("new section not extractable: %v", err)
		}
		if body != "### Added\n\n- new thing" {
			t.Errorf("new section body = %q", body)
		}
		if _, err := Extract(got, "Unreleased"); err == nil {
			t.Error("Unreleased section still has entries, want it emptied")
		}
		if !strings.Contains(got, "## [Unreleased]") {
			t.Error("Unreleased heading was removed")
		}
	})

	t.Run("dates the new heading", func(t *testing.T) {
		if !strings.Contains(got, "## [v0.4.6] - 2026-08-18") {
			t.Errorf("missing dated heading in:\n%s", got)
		}
	})

	t.Run("adds compare link above the previous one", func(t *testing.T) {
		want := "[v0.4.6]: https://example.test/compare/v0.4.5...v0.4.6"
		if !strings.Contains(got, want) {
			t.Fatalf("missing compare link %q", want)
		}
		if strings.Index(got, want) > strings.Index(got, "[v0.4.5]: ") {
			t.Error("new compare link placed below the previous release link")
		}
	})

	t.Run("leaves earlier sections untouched", func(t *testing.T) {
		body, err := Extract(got, "v0.4.5")
		if err != nil || body != "### Fixed\n\n- old thing" {
			t.Errorf("previous section changed: %q (%v)", body, err)
		}
	})
}

func TestCutErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		version string
	}{
		{name: "empty unreleased section", content: emptyUnreleased, version: "v0.4.6"},
		{name: "no unreleased section", content: "# Changelog\n\n## [v0.4.5] - 2026-06-27\n\n- x\n", version: "v0.4.6"},
		{name: "version already released", content: sample, version: "v0.4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Cut(tt.content, tt.version, releaseDate, "https://example.test"); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCutPreservesLineEndings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		crlf    bool
	}{
		{name: "lf stays lf", content: sample},
		{name: "crlf stays crlf", content: strings.ReplaceAll(sample, "\n", "\r\n"), crlf: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Cut(tt.content, "v0.4.6", releaseDate, "https://example.test")
			if err != nil {
				t.Fatalf("Cut returned error: %v", err)
			}
			crlfCount := strings.Count(got, "\r\n")
			if tt.crlf && crlfCount != strings.Count(got, "\n") {
				t.Errorf("mixed endings: %d CRLF of %d LF", crlfCount, strings.Count(got, "\n"))
			}
			if !tt.crlf && crlfCount != 0 {
				t.Errorf("introduced %d CRLF into an LF file", crlfCount)
			}
		})
	}
}

func TestCutWithoutExistingLinkBlock(t *testing.T) {
	content := "# Changelog\n\n## [Unreleased]\n\n- first feature\n"

	got, err := Cut(content, "v0.1.0", releaseDate, "https://example.test/")
	if err != nil {
		t.Fatalf("Cut returned error: %v", err)
	}
	// No previous release to compare against, so the link points at the tag.
	if want := "[v0.1.0]: https://example.test/releases/tag/v0.1.0"; !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}
