// Package workflow implements the Orchestrator-Workers + Evaluator-Optimizer
// pipeline described in DESIGN.md: COO analyzes -> PM audits -> COO approves
// -> DEV implements -> QA verifies -> (on blocking FAIL) back to DEV.
package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/LBT-AI/toolnet.tech/internal/client"
	"github.com/LBT-AI/toolnet.tech/internal/config"
)

// ErrMaxRetriesExceeded is returned when QA keeps failing with a blocking
// severity beyond the configured retry budget.
var ErrMaxRetriesExceeded = fmt.Errorf("qa/dev feedback loop exceeded max retries")

// Runner wires together the 4 role clients and runs the pipeline.
type Runner struct {
	coo        client.LLMClient
	pm         client.LLMClient
	dev        client.LLMClient
	qa         client.LLMClient
	strictMode bool

	// OnStep, if set, is invoked after every pipeline step for progress
	// reporting (e.g. printing to the terminal). Safe to leave nil.
	OnStep func(step, output string)
}

// NewRunner builds a Runner from the loaded configuration.
func NewRunner(cfg *config.Config) (*Runner, error) {
	timeout := time.Duration(cfg.Workflow.TimeoutSeconds) * time.Second
	cooClient, err := client.NewClientForRole(cfg.COO, timeout)
	if err != nil {
		return nil, fmt.Errorf("build COO client: %w", err)
	}
	pmClient, err := client.NewClientForRole(cfg.PM, timeout)
	if err != nil {
		return nil, fmt.Errorf("build PM client: %w", err)
	}
	devClient, err := client.NewClientForRole(cfg.DEV, timeout)
	if err != nil {
		return nil, fmt.Errorf("build DEV client: %w", err)
	}
	qaClient, err := client.NewClientForRole(cfg.QA, timeout)
	if err != nil {
		return nil, fmt.Errorf("build QA client: %w", err)
	}
	return &Runner{
		coo:        cooClient,
		pm:         pmClient,
		dev:        devClient,
		qa:         qaClient,
		strictMode: cfg.Workflow.StrictMode,
	}, nil
}

func (r *Runner) report(step, output string) {
	if r.OnStep != nil {
		r.OnStep(step, output)
	}
}

// Run executes the full pipeline for a single task and returns the final
// state (whether it ended in PASS, or stopped due to retries/errors).
func (r *Runner) Run(ctx context.Context, s *State) error {
	if s.CodeDiff != "" && strings.EqualFold(s.QAResult.Status, "PASS") {
		return nil
	}
	// Resume logic: if we already have a saved state, skip completed steps
	if s.COOPlan == "" {
		// Step 1: COO analyzes and breaks down the task.
		cooPlan, err := r.coo.Call(ctx, cooAnalyzePrompt(), s.OriginalTask)
		if err != nil {
			return fmt.Errorf("step COO_ANALYZE: %w", err)
		}
		s.COOPlan = cooPlan
		s.Record("COO_ANALYZE", s.OriginalTask, cooPlan)
		r.report("COO_ANALYZE", cooPlan)
		if err := s.Save(); err != nil {
			return err
		}
	}

	if s.PMAudit == "" {
		// Step 2: PM audits the plan for risk (no code changes).
		pmAudit, err := r.pm.Call(ctx, pmAuditPrompt(), s.COOPlan)
		if err != nil {
			return fmt.Errorf("step PM_AUDIT: %w", err)
		}
		s.PMAudit = pmAudit
		s.Record("PM_AUDIT", s.COOPlan, pmAudit)
		r.report("PM_AUDIT", pmAudit)
		if err := s.Save(); err != nil {
			return err
		}
	}

	if s.ApprovedPlan == "" {
		// Step 3: COO approves/adjusts the plan based on PM's audit.
		approveInput := fmt.Sprintf("KẾ HOẠCH BAN ĐẦU:\n%s\n\nAUDIT CỦA PM:\n%s", s.COOPlan, s.PMAudit)
		approvedPlan, err := r.coo.Call(ctx, cooApprovePrompt(), approveInput)
		if err != nil {
			return fmt.Errorf("step COO_APPROVE: %w", err)
		}
		s.ApprovedPlan = approvedPlan
		s.Record("COO_APPROVE", approveInput, approvedPlan)
		r.report("COO_APPROVE", approvedPlan)
		if err := s.Save(); err != nil {
			return err
		}
	}

	// Step 4 + 5 + feedback loop: DEV implements, QA verifies, retry on blocking FAIL.
	return r.implementAndVerifyLoop(ctx, s)
}

// implementAndVerifyLoop runs DEV -> QA repeatedly until PASS, a
// non-blocking FAIL under strict mode, or MaxRetries is exceeded.
func (r *Runner) implementAndVerifyLoop(ctx context.Context, s *State) error {
	for {
		var devOutput string
		var err error
		if s.RetryCount == 0 {
			devOutput, err = r.dev.Call(ctx, devImplementPrompt(), s.ApprovedPlan)
		} else {
			devOutput, err = r.dev.Call(ctx, devRetryPrompt(s.CodeDiff, s.QAResult), "")
		}
		if err != nil {
			return fmt.Errorf("step DEV_IMPLEMENT (retry %d): %w", s.RetryCount, err)
		}
		s.CodeDiff = devOutput
		s.Record("DEV_IMPLEMENT", s.ApprovedPlan, devOutput)
		r.report("DEV_IMPLEMENT", devOutput)
		if err := s.Save(); err != nil {
			return err
		}

		// QA verifies the diff WITHOUT seeing the original task/prompt,
		// per DESIGN.md section 1 (avoid intent bias).
		qaRaw, err := r.qa.Call(ctx, qaVerifyPrompt(), s.CodeDiff)
		if err != nil {
			return fmt.Errorf("step QA_VERIFY (retry %d): %w", s.RetryCount, err)
		}
		qaResult := parseQAResponse(qaRaw)
		s.QAResult = qaResult
		s.Record("QA_VERIFY", s.CodeDiff, qaRaw)
		r.report("QA_VERIFY", qaRaw)
		if err := s.Save(); err != nil {
			return err
		}

		if !qaResult.IsFailing() {
			return nil // PASS --- pipeline complete.
		}

		if !qaResult.IsBlockingSeverity() && !r.strictMode {
			// Low/Medium severity: still auto-loop by default (per
			// DESIGN.md section 2), unless strict mode is enabled.
		} else if !qaResult.IsBlockingSeverity() && r.strictMode {
			// Strict mode: stop and let the human decide for non-blocking
			// findings instead of looping automatically.
			return fmt.Errorf("qa returned non-blocking FAIL (severity=%s); strict mode requires manual review", qaResult.Severity)
		}

		s.RetryCount++
		if s.RetryCount > s.MaxRetries {
			return ErrMaxRetriesExceeded
		}
	}
}

var (
	statusRe   = regexp.MustCompile(`(?i)STATUS:\s*(PASS|FAIL)`)
	severityRe = regexp.MustCompile(`(?i)SEVERITY:\s*([A-Za-z]+)`)
	// FINDINGS is the last field, so capture the rest of the text, but stop
	// at the first blank line so trailing conversational text from the model
	// is not swept into the findings.
	findingsRe = regexp.MustCompile(`(?is)FINDINGS:\s*(.*?)(?:\n\s*\n|$)`)
)

// knownSeverities normalizes the many ways a model might spell a severity
// level into the canonical set used by IsBlockingSeverity.
var knownSeverities = map[string]string{
	"critical": "Critical",
	"high":     "High",
	"medium":   "Medium",
	"moderate": "Medium",
	"low":      "Low",
	"none":     "None",
	"":         "None",
}

func normalizeSeverity(s string) string {
	if v, ok := knownSeverities[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v
	}
	// Unknown label: keep it as-is (still non-empty so it is not silently
	// treated as a non-blocking PASS).
	return strings.TrimSpace(s)
}

// parseQAResponse extracts the structured STATUS/SEVERITY/FINDINGS fields
// from the QA model's free-text reply. If the model didn't follow the
// format exactly, it defaults to a FAIL/Critical result so a malformed
// response never gets silently treated as a PASS.
func parseQAResponse(raw string) QAResult {
	result := QAResult{
		Status:   "FAIL",
		Severity: "Critical",
		Findings: strings.TrimSpace(raw),
	}
	if m := statusRe.FindStringSubmatch(raw); m != nil {
		result.Status = strings.ToUpper(m[1])
	}
	if m := severityRe.FindStringSubmatch(raw); m != nil {
		result.Severity = normalizeSeverity(m[1])
	}
	if m := findingsRe.FindStringSubmatch(raw); m != nil {
		result.Findings = strings.TrimSpace(m[1])
	}
	if result.Status == "PASS" {
		result.Severity = ""
	}
	return result
}
