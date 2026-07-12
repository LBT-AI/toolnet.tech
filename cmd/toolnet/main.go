// Command toolnet is the entry point for the multi-agent workflow CLI.
// See README.md for the full pipeline description and usage examples.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LBT-AI/toolnet.tech/brand"
	"github.com/LBT-AI/toolnet.tech/internal/auth"
	"github.com/LBT-AI/toolnet.tech/internal/config"
	gitutil "github.com/LBT-AI/toolnet.tech/internal/git"
	"github.com/LBT-AI/toolnet.tech/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	flagTask       string
	flagTaskID     string
	flagMaxRetries int
	flagBypass     bool
	flagConfigPath string
	flagNoGit      bool
	flagStrict     bool
	flagProvider   string
	flagPort       int
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "toolnet",
		Short: "TOOLNET --- Multi-Agent (COO/PM/DEV/QA) software workflow CLI",
		Long: `TOOLNET CLI lets you interact with AI agents directly from your terminal
		to write, review, and modify code. It orchestrates 4 AI models (COO, PM, DEV, QA)
		in a professional pipeline with automatic feedback loops.`,
		Version: version,
		// No subcommand -> start an interactive session (see docs: "Run
		// interactive session").
		RunE: runInteractive,
	}

	// --config is persistent so it works for the interactive session as well
	// as every subcommand.
	rootCmd.PersistentFlags().StringVar(&flagConfigPath, "config", "", "Path to config.yaml")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full COO -> PM -> COO -> DEV -> QA pipeline for a new task",
		RunE:  runRun,
	}
	runCmd.Flags().StringVar(&flagTask, "task", "", "Task description (required)")
	runCmd.Flags().IntVar(&flagMaxRetries, "max-retries", 0, "Override max QA/DEV retries (0 = use config default)")
	runCmd.Flags().BoolVar(&flagBypass, "bypass", false, "Skip manual approval prompts and run fully automatically")
	runCmd.Flags().BoolVar(&flagStrict, "strict", false, "Enable strict mode: stop on non-blocking QA failures for manual review")
	runCmd.Flags().BoolVar(&flagNoGit, "no-git", false, "Disable git branch creation and automatic patch application")
	_ = runCmd.MarkFlagRequired("task")

	resumeCmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume a previously interrupted session by task ID",
		RunE:  runResume,
	}
	resumeCmd.Flags().StringVar(&flagTaskID, "task-id", "", "Task ID of the session to resume (required)")
	resumeCmd.Flags().BoolVar(&flagBypass, "bypass", false, "Skip manual approval prompts")
	resumeCmd.Flags().BoolVar(&flagStrict, "strict", false, "Enable strict mode")
	resumeCmd.Flags().BoolVar(&flagNoGit, "no-git", false, "Disable automatic patch application")
	_ = resumeCmd.MarkFlagRequired("task-id")

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a provider via OAuth (e.g. openai, antigravity)",
		RunE:  runLogin,
	}
	loginCmd.Flags().StringVar(&flagProvider, "provider", "", "Provider name to authenticate with (required)")
	loginCmd.Flags().IntVar(&flagPort, "port", 1455, "Local callback port for OAuth")
	_ = loginCmd.MarkFlagRequired("provider")

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show current configuration path and validation status",
		RunE:  runConfig,
	}
	configCmd.Flags().StringVar(&flagConfigPath, "config", "", "Path to config.yaml")

	rootCmd.AddCommand(runCmd, resumeCmd, loginCmd, configCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newTaskID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Role colors for step headers (ANSI, no external dependency):
// COO = ocean blue, PM = green, DEV = yellow, QA = red.
const (
	ansiReset  = "\033[0m"
	ansiBlue   = "\033[34m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
)

// stepColor returns the ANSI color for a pipeline step based on its role
// prefix (COO/PM/DEV/QA).
func stepColor(step string) string {
	switch {
	case strings.HasPrefix(step, "COO"):
		return ansiBlue
	case strings.HasPrefix(step, "PM"):
		return ansiGreen
	case strings.HasPrefix(step, "DEV"):
		return ansiYellow
	case strings.HasPrefix(step, "QA"):
		return ansiRed
	default:
		return ""
	}
}

// printStep writes a colored step header followed by the step output.
func printStep(step, output string) {
	c := stepColor(step)
	if c == "" {
		fmt.Printf("=== %s ===\n%s\n\n", step, output)
		return
	}
	fmt.Printf("%s=== %s ===%s\n%s\n\n", c, step, ansiReset, output)
}

// termWidth returns the terminal width (from $COLUMNS or a sane default) so
// chat frames wrap nicely in any terminal emulator (incl. Termius).
func termWidth() int {
	if w := os.Getenv("COLUMNS"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n >= 40 {
			return n
		}
	}
	return 100
}

// wrapLines word-wraps text to the given content width, preserving explicit
// newlines and blank lines.
func wrapLines(s string, width int) []string {
	out := make([]string, 0)
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, w := range strings.Fields(para) {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= width {
				line += " " + w
			} else {
				out = append(out, line)
				line = w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// printChat renders a step as a bordered "chat bubble" with a colored role
// label in the title bar, e.g.:
//
//	┌─ COO_ANALYZE ─────────────────────┐
//	│ <wrapped agent output>             │
//	└────────────────────────────────────┘
func printChat(step, output string) {
	c := stepColor(step)
	width := termWidth()
	if width < 40 {
		width = 40
	}
	bodyW := width - 4
	label := step
	if len(label) > bodyW {
		label = label[:bodyW]
	}
	pad := width - len(label) - 5 // ┌─ + space + space + ┐
	if pad < 1 {
		pad = 1
	}
	if c == "" {
		fmt.Printf("┌─ %s %s┐\n", label, strings.Repeat("─", pad))
	} else {
		fmt.Printf("%s┌─ %s %s┐%s\n", c, label, strings.Repeat("─", pad), ansiReset)
	}
	for _, l := range wrapLines(output, bodyW) {
		if c == "" {
			fmt.Printf("│ %-*s │\n", bodyW, l)
		} else {
			fmt.Printf("%s│ %-*s │%s\n", c, bodyW, l, ansiReset)
		}
	}
	if c == "" {
		fmt.Printf("└%s┘\n\n", strings.Repeat("─", width-2))
	} else {
		fmt.Printf("%s└%s┘%s\n\n", c, strings.Repeat("─", width-2), ansiReset)
	}
}

// runInteractive starts a REPL: read a task from stdin, run the full
// pipeline, print each step, and loop until the user types exit/quit or
// sends EOF. This is the "toolnet" no-subcommand entry point.
func runInteractive(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(flagConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// The pipeline clients are built lazily: on a fresh machine with no API
	// keys configured, we still want to show the banner and start the
	// session. A missing-credential error is reported when a task is run
	// (or after `toolnet login`), not at startup.
	var runner *workflow.Runner
	buildRunner := func() error {
		r, bErr := workflow.NewRunner(cfg)
		if bErr != nil {
			return bErr
		}
		runner = r
		return nil
	}
	if err := buildRunner(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		fmt.Fprintln(os.Stderr, "The banner and session still work; run `toolnet login --provider openai` or set api_key in your config to run tasks.")
	}
	reader := bufio.NewReader(os.Stdin)
	brand.PrintBannerWithTagline("TOOLNET — multi-agent COO/PM/DEV/QA workflow")
	fmt.Println("TOOLNET CLI interactive session.")
	fmt.Println("Type a task and press Enter to run the COO/PM/DEV/QA pipeline.")
	fmt.Println("Type 'exit' or 'quit' (or Ctrl-D) to leave.")
	for {
		fmt.Print("\ntoolnet> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF (Ctrl-D) or read error: leave the session.
			fmt.Println()
			break
		}
		task := strings.TrimSpace(line)
		if task == "" {
			continue
		}
		if task == "exit" || task == "quit" {
			break
		}
		// Rebuild the runner on each task so credentials added via
		// `toolnet login` take effect without restarting the session.
		if runner == nil {
			if bErr := buildRunner(); bErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", bErr)
				continue
			}
		}
		runTaskInline(runner, cfg, task)
	}
	return nil
}

// runTaskInline executes one task through the pipeline and reports the
// outcome, reusing the same logic as `toolnet run` (git branch, step
// printing, diff application) without the interactive approval prompts.
func runTaskInline(runner *workflow.Runner, cfg *config.Config, task string) {
	taskID := newTaskID()
	state := workflow.NewState(taskID, task, cfg.Workflow.MaxRetries)
	if cfg.Workflow.GitAutoBranch && !flagNoGit {
		if err := gitutil.AutoBranch(taskID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create git branch: %v\n", err)
		} else {
			fmt.Printf("Created git branch task/%s\n\n", taskID)
		}
	}
	runner.OnStep = func(step, output string) {
		printChat(step, output)
	}
	ctx := context.Background()
	runErr := runner.Run(ctx, state)
	if runErr != nil {
		if errors.Is(runErr, workflow.ErrMaxRetriesExceeded) {
			fmt.Fprintf(os.Stderr, "\nQA kept failing with blocking severity after %d retries.\n", state.MaxRetries)
			if cfg.Workflow.GitAutoBranch && !flagNoGit && state.QAResult.Severity == "Critical" {
				fmt.Fprintln(os.Stderr, "Severity is Critical --- rolling back working tree changes automatically.")
				if err := gitutil.Rollback(); err != nil {
					fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
				}
			}
			fmt.Fprintf(os.Stderr, "Session saved. Resume/inspect with: toolnet resume --task-id %s\n", taskID)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		}
		return
	}
	if err := applyFinalDiff(state, taskID, cfg, flagNoGit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not apply code changes automatically: %v\n", err)
	}
	fmt.Println("Pipeline finished: QA PASS.")
}

func runRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(flagConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	maxRetries := cfg.Workflow.MaxRetries
	if flagMaxRetries > 0 {
		maxRetries = flagMaxRetries
	}
	if flagStrict {
		cfg.Workflow.StrictMode = true
	}

	taskID := newTaskID()
	state := workflow.NewState(taskID, flagTask, maxRetries)
	fmt.Printf("Task ID: %s\n", taskID)
	fmt.Printf("Task: %s\n\n", flagTask)

	runner, err := workflow.NewRunner(cfg)
	if err != nil {
		return fmt.Errorf("init workflow runner: %w", err)
	}

	if cfg.Workflow.GitAutoBranch && !flagNoGit {
		if err := gitutil.AutoBranch(taskID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create git branch automatically: %v\n", err)
		} else {
			fmt.Printf("Created git branch task/%s\n\n", taskID)
		}
	}

	runner.OnStep = func(step, output string) {
		printStep(step, output)
		if !flagBypass {
			fmt.Println("(--bypass not set: review the output above before the pipeline continues)")
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
		}
	}

	ctx := context.Background()
	runErr := runner.Run(ctx, state)
	if runErr != nil {
		if errors.Is(runErr, workflow.ErrMaxRetriesExceeded) {
			fmt.Fprintf(os.Stderr, "\nQA kept failing with blocking severity after %d retries.\n", state.MaxRetries)
			if cfg.Workflow.GitAutoBranch && !flagNoGit && state.QAResult.Severity == "Critical" {
				fmt.Fprintln(os.Stderr, "Severity is Critical --- rolling back working tree changes automatically.")
				if err := gitutil.Rollback(); err != nil {
					fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
				}
			}
			fmt.Fprintf(os.Stderr, "Session saved. Resume/inspect with: toolnet resume --task-id %s\n", taskID)
			return runErr
		}
		return fmt.Errorf("pipeline failed: %w", runErr)
	}
	if err := applyFinalDiff(state, taskID, cfg, flagNoGit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not apply code changes automatically: %v\n", err)
	}
	fmt.Println("Pipeline finished: QA PASS. Review the diff above before merging.")
	return nil
}

// applyFinalDiff writes the DEV agent's final diff to the working tree. If
// the patch does not apply cleanly (e.g. the model produced an invalid or
// incremental diff), it is saved to a file for manual application instead of
// failing the run.
func applyFinalDiff(state *workflow.State, taskID string, cfg *config.Config, noGit bool) error {
	if state.CodeDiff == "" {
		return nil
	}
	if noGit || !cfg.Workflow.GitAutoBranch {
		// Without git automation we cannot safely apply; leave the raw diff
		// for the user. This is not an error.
		return nil
	}
	if err := gitutil.ApplyDiff(state.CodeDiff); err != nil {
		path, saveErr := workflow.SavePatch(taskID, state.CodeDiff)
		if saveErr != nil {
			return fmt.Errorf("apply patch: %v; save fallback: %w", err, saveErr)
		}
		return fmt.Errorf("apply patch: %v (saved for manual use at %s)", err, path)
	}
	return nil
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(flagConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if flagStrict {
		cfg.Workflow.StrictMode = true
	}
	state, err := workflow.LoadState(flagTaskID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", flagTaskID, err)
	}
	fmt.Printf("Resumed session %s (retry count: %d/%d)\n", state.TaskID, state.RetryCount, state.MaxRetries)
	for _, h := range state.History {
		fmt.Printf("[%s] %s\n", h.Timestamp.Format("2006-01-02 15:04:05"), h.Step)
	}

	runner, err := workflow.NewRunner(cfg)
	if err != nil {
		return fmt.Errorf("init workflow runner: %w", err)
	}
	runner.OnStep = func(step, output string) {
		printStep(step, output)
		if !flagBypass {
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
		}
	}

	ctx := context.Background()
	if err := runner.Run(ctx, state); err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}
	if err := applyFinalDiff(state, flagTaskID, cfg, flagNoGit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not apply code changes automatically: %v\n", err)
	}
	fmt.Println("Pipeline finished: QA PASS.")
	return nil
}

func runLogin(cmd *cobra.Command, args []string) error {
	fmt.Printf("Starting OAuth login for provider: %s\n", flagProvider)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	store, err := auth.NewCredentialStore()
	if err != nil {
		return err
	}

	if auth.IsDeviceFlowProvider(flagProvider) {
		return runLoginDevice(ctx, store)
	}
	return runLoginCode(ctx, store)
}

// runLoginDevice handles providers that use the OAuth2 Device Authorization
// flow (OpenAI Codex, Gemini, Antigravity, ...): the user visits a URL and
// enters a short code, and the CLI polls for the token.
func runLoginDevice(ctx context.Context, store *auth.CredentialStore) error {
	flow, err := auth.StartDeviceAuthorization(ctx, flagProvider)
	if err != nil {
		return fmt.Errorf("start device authorization: %w", err)
	}
	fmt.Println("To authenticate, open the following URL and enter the code shown:")
	fmt.Printf("\n  %s\n  (code: %s)\n\n", flow.VerificationURI, flow.UserCode)
	fmt.Println("Waiting for you to approve in the browser...")

	creds, err := flow.Poll(ctx)
	if err != nil {
		return fmt.Errorf("device flow: %w", err)
	}
	if err := store.Save(*creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	printSavedCredential(flagProvider, creds)
	return nil
}

// runLoginCode handles providers that use the browser redirect
// (authorization-code) flow with a temporary local callback server.
func runLoginCode(ctx context.Context, store *auth.CredentialStore) error {
	// This is a generic OAuth scaffold. Real URLs would come from provider docs.
	authURL := fmt.Sprintf("https://auth.%s.com/oauth/authorize?client_id=YOUR_CLIENT_ID&redirect_uri=http://localhost:%d/auth/callback&response_type=code&scope=openid+profile+email", flagProvider, flagPort)
	fmt.Printf("Auth URL: %s\n", authURL)

	server := auth.NewOAuthServer(flagPort)
	callbackURL := server.Start()
	fmt.Printf("Waiting for callback at %s ...\n", callbackURL)

	code, err := server.Wait(ctx)
	if err != nil {
		return fmt.Errorf("oauth login failed: %w", err)
	}
	fmt.Printf("Received authorization code: %s\n", maskToken(code))

	creds, err := auth.ExchangeCode(ctx, flagProvider, code, callbackURL)
	if err != nil {
		return fmt.Errorf("exchange authorization code: %w", err)
	}
	if err := store.Save(*creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	printSavedCredential(flagProvider, creds)
	return nil
}

// printSavedCredential prints a short confirmation after a successful login.
func printSavedCredential(provider string, creds *auth.Credentials) {
	fmt.Printf("Credentials for provider %q saved securely", provider)
	if !creds.ExpiresAt.IsZero() {
		fmt.Printf(" (expires %s)", creds.ExpiresAt.Format(time.RFC3339))
	}
	fmt.Println(".")
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(flagConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	fmt.Println("TOOLNET Configuration loaded successfully.")
	store, _ := auth.NewCredentialStore()
	printRole("COO", cfg.COO, store)
	printRole("PM ", cfg.PM, store)
	printRole("DEV", cfg.DEV, store)
	printRole("QA ", cfg.QA, store)
	fmt.Printf("Workflow: max_retries=%d strict_mode=%v git_auto_branch=%v timeout=%ds\n",
		cfg.Workflow.MaxRetries, cfg.Workflow.StrictMode, cfg.Workflow.GitAutoBranch, cfg.Workflow.TimeoutSeconds)
	return nil
}

// printRole reports a role's providers and how many accounts will be rotated
// (configured api_keys per account plus any stored OAuth logins), and
// whether the role runs single or round-robin.
func printRole(label string, role config.RoleConfig, store *auth.CredentialStore) {
	agents := role.Agents()
	total := 0
	providers := map[string]int{}
	weighted := false
	for _, a := range agents {
		if a.Disabled {
			continue
		}
		n := len(a.APIKeys)
		if n == 0 && a.APIKey != "" {
			n = 1
		}
		if store != nil {
			if creds, err := store.ListCredentials(a.Provider); err == nil {
				n += len(creds)
			}
		}
		total += n
		providers[a.Provider]++
		if a.Weight > 1 {
			weighted = true
		}
	}
	mode := "single"
	if total > 1 {
		if role.Sticky {
			mode = "sticky"
		} else {
			mode = "round-robin"
		}
	}
	if weighted {
		mode += "+weighted"
	}
	ps := make([]string, 0, len(providers))
	for p := range providers {
		ps = append(ps, p)
	}
	fmt.Printf("%s:   providers=%v  (accounts=%d, %s)\n", label, ps, total, mode)
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
