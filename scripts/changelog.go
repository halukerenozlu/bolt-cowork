//go:build ignore

// changelog.go is the command line front end for tools/changelog.
//
// Usage (via Makefile):
//
//	go run ./scripts/changelog.go cut v0.4.6      # release prep, edits CHANGELOG.md
//	go run ./scripts/changelog.go extract v0.4.6  # release notes, prints to stdout
//
// "cut" is run by hand on dev before opening the release pull request; it turns
// the Unreleased section into a dated section for the given version. "extract"
// is run by .github/workflows/release.yml on tag push to build the GitHub
// release notes from that same section.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/halukerenozlu/bolt-cowork/tools/changelog"
)

const (
	changelogPath = "CHANGELOG.md"
	repoURL       = "https://github.com/halukerenozlu/bolt-cowork"
)

func main() {
	if len(os.Args) != 3 {
		fail("usage: go run ./scripts/changelog.go <cut|extract> <version>")
	}
	command, version := os.Args[1], os.Args[2]

	content, err := os.ReadFile(changelogPath)
	if err != nil {
		fail("read %s: %v", changelogPath, err)
	}

	switch command {
	case "cut":
		out, err := changelog.Cut(string(content), version, time.Now(), repoURL)
		if err != nil {
			fail("cut %s: %v", version, err)
		}
		if err := os.WriteFile(changelogPath, []byte(out), 0o644); err != nil {
			fail("write %s: %v", changelogPath, err)
		}
		fmt.Fprintf(os.Stderr, "%s: cut %s\n", changelogPath, version)
	case "extract":
		body, err := changelog.Extract(string(content), version)
		if err != nil {
			fail("extract %s: %v", version, err)
		}
		fmt.Println(body)
	default:
		fail("unknown command %q, expected cut or extract", command)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "changelog: "+format+"\n", args...)
	os.Exit(1)
}
