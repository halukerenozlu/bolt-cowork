package tool

import (
	"slices"
	"testing"
)

func TestListOutputRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		entries []string
	}{
		{name: "ordinary entries", path: ".", entries: []string{"file-a", "subdir/"}},
		{name: "comma in filename", path: ".", entries: []string{"report, final.pdf", "other.txt"}},
		{name: "empty directory", path: "empty", entries: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := FormatListOutput(tt.path, tt.entries)
			path, entries, ok := ParseListOutput(raw)
			if !ok || path != tt.path || !slices.Equal(entries, tt.entries) {
				t.Fatalf("round trip = path:%q entries:%v ok:%v, want path:%q entries:%v", path, entries, ok, tt.path, tt.entries)
			}
		})
	}
}
