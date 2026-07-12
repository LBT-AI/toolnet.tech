package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var safeTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// SavePatch preserves a patch that could not be applied automatically.
func SavePatch(taskID, patch string) (string, error) {
	if !safeTaskID.MatchString(taskID) {
		return "", fmt.Errorf("invalid task id %q", taskID)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".toolnet", "patches")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create patch dir: %w", err)
	}
	path := filepath.Join(dir, taskID+".patch")
	if err := os.WriteFile(path, []byte(patch), 0o600); err != nil {
		return "", fmt.Errorf("write patch: %w", err)
	}
	return path, nil
}
