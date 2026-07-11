// Package git wraps the minimal set of git CLI commands the workflow
// needs: creating an isolated branch per task, and rolling back if QA
// reports a Critical failure after all retries are exhausted. Kept as a
// thin os/exec wrapper on purpose --- no external git library dependency.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v failed: %w (stderr: %s)", args, err, stderr.String())
	}
	return stdout.String(), nil
}

// isGitRepo reports whether the current directory is inside a git work tree.
func isGitRepo() bool {
	_, err := run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// isClean reports whether the working tree has no uncommitted changes.
func isClean() (bool, error) {
	out, err := run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// branchExists reports whether a local branch with the given name exists.
func branchExists(branch string) (bool, error) {
	_, err := run("rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		// git exits non-zero when the branch is absent; treat that as "not found".
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// AutoBranch creates and checks out an isolated branch for the given
// task ID, e.g. "task/1a2b3c". It refuses to proceed on a dirty working
// tree so in-flight user changes are never swept into the task branch, and
// falls back to a timestamped suffix if the branch name is already taken
// (e.g. a resumed task).
func AutoBranch(taskID string) error {
	if !isGitRepo() {
		return fmt.Errorf("current directory is not a git repository")
	}
	clean, err := isClean()
	if err != nil {
		return fmt.Errorf("check working tree: %w", err)
	}
	if !clean {
		return fmt.Errorf("working tree is not clean; commit or stash your changes before starting a task")
	}
	branch := fmt.Sprintf("task/%s", taskID)
	exists, err := branchExists(branch)
	if err != nil {
		return err
	}
	if exists {
		branch = fmt.Sprintf("task/%s-%d", taskID, time.Now().UnixNano())
	}
	if _, err := run("checkout", "-b", branch); err != nil {
		return fmt.Errorf("create task branch %q: %w", branch, err)
	}
	return nil
}

// ApplyDiff applies a unified diff to the working tree. It first validates
// the patch with `git apply --check` so a malformed or rejected hunk fails
// fast with a clear error instead of partially modifying files.
func ApplyDiff(diff string) error {
	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("empty diff")
	}
	cmd := exec.Command("git", "apply", "--check", "--whitespace=nowarn", "-")
	cmd.Stdin = strings.NewReader(diff)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("diff does not apply cleanly: %w (output: %s)", err, string(out))
	}
	apply := exec.Command("git", "apply", "--whitespace=nowarn", "-")
	apply.Stdin = strings.NewReader(diff)
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// Rollback discards all uncommitted changes in the working tree. Intended
// to be called only after QA reports Critical and MaxRetries has been
// exceeded (see workflow.ErrMaxRetriesExceeded).
func Rollback() error {
	if _, err := run("reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("git reset --hard: %w", err)
	}
	if _, err := run("clean", "-fd"); err != nil {
		return fmt.Errorf("git clean -fd: %w", err)
	}
	return nil
}

// CurrentBranch returns the name of the currently checked-out branch.
func CurrentBranch() (string, error) {
	out, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return trimNewline(out), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
