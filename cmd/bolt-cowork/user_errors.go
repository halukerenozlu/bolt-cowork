package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

const maxPathSuggestions = 5

var errStopWalk = errors.New("stop walk")

// printRunError prints a user-friendly message when possible, and falls back
// to the full technical error for cases we don't specially handle.
// If redactor is non-nil, error text is redacted before printing.
func printRunError(err error, command string, cfg *config.Config, redactor *agent.Redactor) {
	if printFriendlyNotFoundError(err, command, cfg) {
		return
	}
	msg := err.Error()
	if redactor != nil {
		msg = redactor.Redact(msg)
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
}

func printFriendlyNotFoundError(err error, command string, cfg *config.Config) bool {
	// os.IsNotExist should catch wrapped *os.PathError values, but some
	// platform-specific messages can still slip through heavily wrapped chains.
	msg := strings.ToLower(err.Error())
	if !os.IsNotExist(err) &&
		!strings.Contains(msg, "no such file or directory") &&
		!strings.Contains(msg, "cannot find the file specified") {
		return false
	}

	root := resolveWorkDir(cfg)
	absRoot, absErr := filepath.Abs(root)
	if absErr == nil {
		root = absRoot
	}

	missing := missingPathFromError(err)
	if missing == "" {
		missing = extractLikelyPathFromCommand(command)
	}
	if missing == "" {
		fmt.Fprintln(os.Stderr, "Couldn't find the requested file or folder.")
		fmt.Fprintln(os.Stderr, `Try a path relative to the workspace (for example: "test6/merhaba.txt").`)
		return true
	}

	display := prettyPath(missing, root)
	fmt.Fprintf(os.Stderr, "Couldn't find: %q\n", display)

	targetName := filepath.Base(missing)
	if targetName == "." || targetName == string(filepath.Separator) {
		fmt.Fprintln(os.Stderr, `Try a path relative to the workspace (for example: "test6/merhaba.txt").`)
		return true
	}

	suggestions := findPathSuggestions(root, targetName, maxPathSuggestions)
	if len(suggestions) == 0 {
		fmt.Fprintln(os.Stderr, `Try a path relative to the workspace (for example: "test6/merhaba.txt").`)
		return true
	}

	fmt.Fprintln(os.Stderr, "Did you mean one of these?")
	for _, s := range suggestions {
		fmt.Fprintf(os.Stderr, "  - %s\n", s)
	}
	return true
}

func missingPathFromError(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Path != "" {
		return pathErr.Path
	}
	return ""
}

func extractLikelyPathFromCommand(command string) string {
	fields := strings.Fields(command)
	for _, f := range fields {
		token := strings.TrimSpace(strings.Trim(f, `"'`))
		if token == "" {
			continue
		}
		// Heuristic for path-like tokens.
		if strings.Contains(token, "/") || strings.Contains(token, `\`) || strings.Contains(token, ".") {
			return token
		}
	}
	return ""
}

func prettyPath(path, root string) string {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func findPathSuggestions(root, targetName string, limit int) []string {
	if root == "" || targetName == "" || limit <= 0 {
		return nil
	}

	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(d.Name(), targetName) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) >= limit {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil
	}
	return out
}
