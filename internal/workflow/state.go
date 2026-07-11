package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// QAResult is the structured verdict returned by the QA role.
type QAResult struct {
	Status   string `json:"status"`   // "PASS" | "FAIL"
	Severity string `json:"severity"` // "Critical" | "High" | "Medium" | "Low" | ""
	Findings string `json:"findings"`
}

// IsFailing reports whether this QA result requires a retry.
func (q QAResult) IsFailing() bool {
	return q.Status == "FAIL"
}

// IsBlockingSeverity reports whether the severity is high enough to
// automatically trigger the DEV feedback loop (Critical/High) per
// DESIGN.md section 2.
func (q QAResult) IsBlockingSeverity() bool {
	return q.Severity == "Critical" || q.Severity == "High"
}

// HistoryEntry records one step of the pipeline for auditing/debugging.
type HistoryEntry struct {
	Step      string    `json:"step"`
	Timestamp time.Time `json:"timestamp"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
}

// State is the single shared object every role reads from / writes to.
// It is the in-code equivalent of the "context.json" described in
// DESIGN.md section 3.
type State struct {
	TaskID       string         `json:"task_id"`
	OriginalTask string         `json:"original_task"`
	COOPlan      string         `json:"coo_plan"`
	PMAudit      string         `json:"pm_audit"`
	ApprovedPlan string         `json:"approved_plan"`
	CodeDiff     string         `json:"code_diff"`
	QAResult     QAResult       `json:"qa_result"`
	RetryCount   int            `json:"retry_count"`
	MaxRetries   int            `json:"max_retries"`
	History      []HistoryEntry `json:"history"`
	CreatedAt    time.Time      `json:"created_at"`
}

// NewState initializes a fresh pipeline state for a new task.
func NewState(taskID, task string, maxRetries int) *State {
	return &State{
		TaskID:       taskID,
		OriginalTask: task,
		MaxRetries:   maxRetries,
		History:      []HistoryEntry{},
		CreatedAt:    time.Now(),
	}
}

// Record appends a step to the audit trail.
func (s *State) Record(step, input, output string) {
	s.History = append(s.History, HistoryEntry{
		Step:      step,
		Timestamp: time.Now(),
		Input:     input,
		Output:    output,
	})
}

func sessionPath(taskID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".lbt", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	return filepath.Join(dir, fmt.Sprintf("session-%s.json", taskID)), nil
}

// Save persists the state to ~/.lbt/sessions/session-<taskID>.json so a
// crashed or interrupted run can be resumed with `toolnet resume`.
func (s *State) Save() error {
	path, err := sessionPath(s.TaskID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

// LoadState reads a previously saved session back into memory.
func LoadState(taskID string) (*State, error) {
	path, err := sessionPath(taskID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file %q: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session file %q: %w", path, err)
	}
	return &s, nil
}
