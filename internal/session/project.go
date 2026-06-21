package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// ProjectKey returns a short, stable identifier for workspace, derived from
// its absolute path. It is used to namespace per-project session storage
// under the global ~/.bolt-cowork/sessions/ directory, mirroring how the
// same workspace always resolves to the same key regardless of working
// directory at the time of the call.
func ProjectKey(workspace string) (string, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("session: resolve workspace path: %w", err)
	}
	norm := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		norm = strings.ToLower(norm)
	}
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:16], nil
}

// DirForWorkspace returns the global session storage directory for
// workspace: <home>/.bolt-cowork/sessions/<project-key>.
func DirForWorkspace(home, workspace string) (string, error) {
	key, err := ProjectKey(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bolt-cowork", "sessions", key), nil
}
