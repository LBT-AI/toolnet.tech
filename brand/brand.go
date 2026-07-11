package brand

import (
	"fmt"
	"strings"
)

const (
	Reset  = "\x1b[0m"
	Bold   = "\x1b[1m"
	Cyan   = "\x1b[38;2;0;200;220m"
	Yellow = "\x1b[38;2;255;200;50m"
	White  = "\x1b[37m"
	Gray   = "\x1b[90m"
	Green  = "\x1b[38;2;50;200;50m"
	Red    = "\x1b[38;2;255;80;80m"
	Orange = "\x1b[38;2;255;150;50m"
	Purple = "\x1b[38;2;180;100;255m"
)

// ── Pixel Art Letters ──
// Each letter is 8 chars wide, 7 rows high, with white borders

var letterT = []string{
	White + "┌──────┐" + Reset,
	White + "│" + Cyan + "██████" + White + "│" + Reset,
	White + "└──────┘" + Reset,
	"  " + White + "│" + Cyan + "██" + White + "│" + Reset + "  ",
	"  " + White + "│" + Cyan + "██" + White + "│" + Reset + "  ",
	"  " + White + "│" + Cyan + "██" + White + "│" + Reset + "  ",
	"  " + White + "└──┘" + Reset + "  ",
}

var letterO = []string{
	White + "┌──────┐" + Reset,
	White + "│" + Cyan + "██████" + White + "│" + Reset,
	White + "│" + Cyan + "██  ██" + White + "│" + Reset,
	White + "│" + Cyan + "██  ██" + White + "│" + Reset,
	White + "│" + Cyan + "██  ██" + White + "│" + Reset,
	White + "│" + Cyan + "██████" + White + "│" + Reset,
	White + "└──────┘" + Reset,
}

var letterL = []string{
	White + "┌" + Reset + "       ",
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "██████" + Reset + " ",
	White + "└──────┘" + Reset,
}

var letterN = []string{
	White + "┌─┐" + Reset + "  " + White + "┌─┐" + Reset,
	White + "│" + Cyan + "█" + Reset + White + "│" + Reset + "  " + White + "│" + Cyan + "█" + Reset + White + "│" + Reset,
	White + "│" + Cyan + "█" + Reset + White + "└─┘" + Reset + " " + White + "│" + Cyan + "█" + Reset + White + "│" + Reset,
	White + "│" + Cyan + "█" + Reset + " " + White + "┌─┘" + Reset + Cyan + "█" + Reset + White + "│" + Reset,
	White + "│" + Cyan + "█" + Reset + White + "│" + Reset + "  " + White + "│" + Cyan + "█" + Reset + White + "│" + Reset,
	White + "│" + Cyan + "█" + Reset + White + "│" + Reset + "  " + White + "│" + Cyan + "█" + Reset + White + "│" + Reset,
	White + "└─┘" + Reset + "  " + White + "└─┘" + Reset,
}

var letterE = []string{
	White + "┌──────┐" + Reset,
	White + "│" + Cyan + "██████" + White + "│" + Reset,
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "█████" + Reset + "  ",
	White + "│" + Cyan + "██" + Reset + "     ",
	White + "│" + Cyan + "██████" + White + "│" + Reset,
	White + "└──────┘" + Reset,
}

// combineLetters joins letter rows side by side with spacing
func combineLetters(letters ...[]string) []string {
	if len(letters) == 0 {
		return nil
	}
	rows := len(letters[0])
	result := make([]string, rows)
	for i := 0; i < rows; i++ {
		var parts []string
		for _, letter := range letters {
			if i < len(letter) {
				parts = append(parts, letter[i])
			}
		}
		result[i] = strings.Join(parts, "  ")
	}
	return result
}

// Header returns the macOS-style header bar
func Header(version string) string {
	if version == "" {
		version = "v1.0.0"
	}
	return fmt.Sprintf(
		"%s●%s  %s●%s  %s●%s    %sTOOLNET CLI%s    %s%s%s\n\n",
		Red, Reset, Orange, Reset, Green, Reset,
		Bold+White, Reset,
		Gray, version, Reset,
	)
}

// Banner returns the full LETAN MEDIA brand banner
func Banner() string {
	var b strings.Builder

	// Welcome line
	b.WriteString(White + "Welcome to " + Reset)
	b.WriteString(Yellow + "LETAN MEDIA" + Reset)
	b.WriteString("\n\n")

	// TOOLNET combined
	toolnetRows := combineLetters(letterT, letterO, letterO, letterL, letterN, letterE, letterT)
	for _, row := range toolnetRows {
		b.WriteString("  " + row + "\n")
	}

	b.WriteString("\n")
	b.WriteString(Gray + "  CLI Version 1.0.0" + Reset)
	b.WriteString("\n\n")

	// Globe + Coin icons
	iconLines := []string{
		"              " + Cyan + "    ████████████████    " + Reset + "  " + Yellow + "    ████████████████    " + Reset,
		"              " + Cyan + "  ████  ██  ██  ████  " + Reset + "  " + Yellow + "  ████   ██████   ████  " + Reset,
		"              " + Cyan + " ████  ████  ████  ████ " + Reset + "  " + Yellow + " ████   ██  ██  ██   ████ " + Reset,
		"              " + Cyan + " ██  ████  ██  ████  ██ " + Reset + "  " + Yellow + " ████  ████  ████  ████ " + Reset,
		"              " + Cyan + "███  ██  ██████  ██  ███" + Reset + "  " + Yellow + " ████  ██████████  ████ " + Reset,
		"              " + Cyan + " ██  ██  ██  ██  ██  ██ " + Reset + "  " + Yellow + " ████  ██████████  ████ " + Reset,
		"              " + Cyan + "  ████  ██  ██  ████  " + Reset + "  " + Yellow + "  ████   ██████   ████  " + Reset,
		"              " + Cyan + "    ████████████████    " + Reset + "  " + Yellow + "    ████████████████    " + Reset,
	}

	for _, line := range iconLines {
		b.WriteString(line + "\n")
	}

	return b.String()
}

// Separator returns a horizontal line
func Separator() string {
	return White + strings.Repeat("─", 76) + Reset + "\n"
}

// WorkflowStep represents one step in the orchestrator workflow
type WorkflowStep struct {
	Number int
	Role   string
	Model  string
	Label  string
	Status string
	Color  string
}

// Workflow returns the orchestrator workflow display
func Workflow(steps []WorkflowStep) string {
	var b strings.Builder
	b.WriteString(Cyan + "ORCHESTRATOR WORKFLOW" + Reset)
	b.WriteString("\n")
	b.WriteString(Separator())

	for i, step := range steps {
		if i > 0 {
			b.WriteString(Gray + "    ↓" + Reset + "\n")
		}
		statusColor := Gray
		if step.Status == "Ready" {
			statusColor = Green
		} else if step.Status == "Waiting" {
			statusColor = Yellow
		} else if step.Status == "Active" {
			statusColor = Cyan
		}

		b.WriteString(fmt.Sprintf(
			"%s[%d]%s  %s%s%s  (%s)  %s→%s  %s%s%s\n",
			White, step.Number, Reset,
			step.Color, step.Role, Reset,
			step.Model,
			Gray, Reset,
			statusColor, step.Status, Reset,
		))
		b.WriteString(fmt.Sprintf("      %s%s%s\n", Gray, step.Label, Reset))
	}

	return b.String()
}

// LiveLogEntry represents a log entry
type LiveLogEntry struct {
	Time   string
	Actor  string
	Action string
	Color  string
}

// LiveLog returns the live log panel
func LiveLog(entries []LiveLogEntry) string {
	var b strings.Builder
	b.WriteString(White + "◉ LIVE LOG" + Reset)
	b.WriteString("\n")
	b.WriteString(Separator())

	for _, entry := range entries {
		b.WriteString(fmt.Sprintf(
			"%s[%s]%s  %s%s%s  %s%s\n",
			Gray, entry.Time, Reset,
			entry.Color, entry.Actor, Reset,
			entry.Action, Reset,
		))
	}

	return b.String()
}

// StatusBar returns the bottom status bar
func StatusBar(user, path, branch, latency string) string {
	return fmt.Sprintf(
		"%s%s@%s%s  %s%s%s  %s(%s)%s  %s|%s  Latency: %s%s\n",
		Cyan, user, Reset,
		White, path, Reset,
		Yellow, branch, Reset,
		Gray, Reset,
		Green, latency, Reset,
	)
}

// Prompt returns the command prompt line
func Prompt() string {
	return fmt.Sprintf("%s>%s Enter a command or %s[@]%s to mention files... %s█%s\n",
		Gray, Reset, Cyan, Reset, White, Reset,
	)
}

// PrintFullUI prints the complete CLI UI
func PrintFullUI(version string, steps []WorkflowStep, logs []LiveLogEntry, user, path, branch, latency string) {
	fmt.Print(Header(version))
	fmt.Print(Banner())
	fmt.Print("\n")
	fmt.Print(Workflow(steps))
	fmt.Print("\n")
	fmt.Print(LiveLog(logs))
	fmt.Print("\n")
	fmt.Print(StatusBar(user, path, branch, latency))
	fmt.Print(Prompt())
}

// PrintBanner prints just the brand banner
func PrintBanner() {
	fmt.Print(Banner())
}

// PrintBannerWithTagline prints banner + tagline
func PrintBannerWithTagline(tagline string) {
	fmt.Print(Banner())
	if tagline != "" {
		fmt.Println(Gray + tagline + Reset)
	}
}
