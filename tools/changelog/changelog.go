// Package changelog reads and edits CHANGELOG.md in Keep a Changelog format.
//
// It backs the two release-time operations: Cut turns the Unreleased section
// into a dated release section (run by hand before a release PR), and Extract
// returns a single release's body (run by the release workflow to build the
// GitHub release notes).
package changelog

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const unreleased = "Unreleased"

// headingRe matches a section heading. Older releases were written without the
// "v" prefix ("## [0.3.5]"), newer ones with it ("## [v0.4.5]"); both are read.
var headingRe = regexp.MustCompile(`^## \[(Unreleased|v?\d+\.\d+\.\d+)\]`)

// linkRefRe matches a compare-link definition in the block at the end of file.
var linkRefRe = regexp.MustCompile(`^\[v?\d+\.\d+\.\d+\]:\s`)

// Extract returns the body of the section for version, without its heading and
// with surrounding blank lines trimmed. version may be given with or without
// the "v" prefix.
func Extract(content, version string) (string, error) {
	lines, _ := splitLines(content)

	start := findHeading(lines, version)
	if start < 0 {
		return "", fmt.Errorf("no changelog section for %s", version)
	}

	body := strings.TrimSpace(strings.Join(lines[start+1:sectionEnd(lines, start)], "\n"))
	if body == "" {
		return "", fmt.Errorf("changelog section for %s is empty", version)
	}
	return body, nil
}

// Cut converts the Unreleased section into a release section for version dated
// date, leaves a fresh empty Unreleased heading above it, and adds a compare
// link against the previous release. repoURL is the repository base URL, e.g.
// "https://github.com/halukerenozlu/bolt-cowork".
func Cut(content, version string, date time.Time, repoURL string) (string, error) {
	lines, crlf := splitLines(content)
	tag := withV(version)

	start := findHeading(lines, unreleased)
	if start < 0 {
		return "", fmt.Errorf("no %s section in changelog", unreleased)
	}

	if strings.TrimSpace(strings.Join(lines[start+1:sectionEnd(lines, start)], "\n")) == "" {
		return "", fmt.Errorf("%s section is empty, nothing to release as %s", unreleased, tag)
	}
	if findHeading(lines, tag) >= 0 {
		return "", fmt.Errorf("changelog already has a section for %s", tag)
	}

	// Previous release heading, if any, follows the Unreleased body.
	prev := ""
	if next := nextHeading(lines, start); next < len(lines) {
		prev = withV(headingRe.FindStringSubmatch(lines[next])[1])
	}

	heading := fmt.Sprintf("## [%s] - %s", tag, date.Format("2006-01-02"))
	out := append([]string{}, lines[:start+1]...)
	out = append(out, "", heading)
	out = append(out, lines[start+1:]...)

	return joinLines(insertLinkRef(out, tag, prev, repoURL), crlf), nil
}

// insertLinkRef adds the compare link for tag at the top of the trailing link
// reference block, which is kept in descending version order.
func insertLinkRef(lines []string, tag, prev, repoURL string) []string {
	repoURL = strings.TrimSuffix(repoURL, "/")

	link := fmt.Sprintf("[%s]: %s/releases/tag/%s", tag, repoURL, tag)
	if prev != "" {
		link = fmt.Sprintf("[%s]: %s/compare/%s...%s", tag, repoURL, prev, tag)
	}

	for i, line := range lines {
		if linkRefRe.MatchString(line) {
			return append(lines[:i:i], append([]string{link}, lines[i:]...)...)
		}
	}

	// No link block yet: start one after a blank separator line.
	out := append([]string{}, lines...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return append(out, "", link, "")
}

// findHeading returns the index of the heading for version, or -1.
func findHeading(lines []string, version string) int {
	for i, line := range lines {
		m := headingRe.FindStringSubmatch(line)
		if m != nil && sameVersion(m[1], version) {
			return i
		}
	}
	return -1
}

// sectionEnd returns where the section opened at start ends: at the next
// heading, or at the trailing link reference block, whichever comes first.
// The link block belongs to no section and must not leak into release notes.
func sectionEnd(lines []string, start int) int {
	for i := start + 1; i < len(lines); i++ {
		if headingRe.MatchString(lines[i]) || linkRefRe.MatchString(lines[i]) {
			return i
		}
	}
	return len(lines)
}

// nextHeading returns the index of the first heading after start, or len(lines).
func nextHeading(lines []string, start int) int {
	for i := start + 1; i < len(lines); i++ {
		if headingRe.MatchString(lines[i]) {
			return i
		}
	}
	return len(lines)
}

func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

func withV(version string) string {
	if version == unreleased || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

// splitLines splits content into lines and reports whether it used CRLF, so
// that Cut can write the file back with the endings it found.
func splitLines(content string) ([]string, bool) {
	if strings.Contains(content, "\r\n") {
		return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n"), true
	}
	return strings.Split(content, "\n"), false
}

func joinLines(lines []string, crlf bool) string {
	out := strings.Join(lines, "\n")
	if crlf {
		return strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out
}
