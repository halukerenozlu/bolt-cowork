package tool

import (
	"encoding/json"
	"strings"
)

const listOutputPrefix = "Listed "

type listOutput struct {
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
}

// FormatListOutput serializes a directory listing without using a filename
// delimiter. JSON preserves commas, quotes, and newlines in valid filenames.
func FormatListOutput(path string, entries []string) string {
	payload, err := json.Marshal(listOutput{Path: path, Entries: entries})
	if err != nil {
		return listOutputPrefix + `{"path":"","entries":[]}`
	}
	return listOutputPrefix + string(payload)
}

// ParseListOutput decodes a value produced by FormatListOutput.
func ParseListOutput(output string) (path string, entries []string, ok bool) {
	if !strings.HasPrefix(output, listOutputPrefix) {
		return "", nil, false
	}
	var result listOutput
	if err := json.Unmarshal([]byte(strings.TrimPrefix(output, listOutputPrefix)), &result); err != nil {
		return "", nil, false
	}
	return result.Path, result.Entries, true
}
