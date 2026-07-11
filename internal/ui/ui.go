package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
)

// ─── Color presets ───
var (
	Cyan        = color.New(color.FgCyan, color.Bold).SprintFunc()
	CyanLight   = color.New(color.FgHiCyan).SprintFunc()
	Yellow      = color.New(color.FgYellow, color.Bold).SprintFunc()
	YellowLight = color.New(color.FgHiYellow).SprintFunc()
	Magenta     = color.New(color.FgMagenta, color.Bold).SprintFunc()
	Green       = color.New(color.FgGreen, color.Bold).SprintFunc()
	GreenLight  = color.New(color.FgHiGreen).SprintFunc()
	Red         = color.New(color.FgRed, color.Bold).SprintFunc()
	RedLight    = color.New(color.FgHiRed).SprintFunc()
	White       = color.New(color.FgWhite, color.Bold).SprintFunc()
	HiWhite     = color.New(color.FgHiWhite).SprintFunc()
	Gray        = color.New(color.FgHiBlack).SprintFunc()
	Blue        = color.New(color.FgBlue, color.Bold).SprintFunc()
)

// ─── Box Drawing ───
const (
	H  = "─"
	V  = "│"
	TL = "┌"
	TR = "┐"
	BL = "└"
	BR = "┘"
	ML = "├"
	MR = "┤"
	TM = "┬"
	BM = "┴"
	MM = "┼"
)

// ─── Splash Screen ───
func PrintSplash(version string) {
	fmt.Println()
	fmt.Println(Cyan(ToolnetASCIIArt()))
	fmt.Println()
	fmt.Printf("  %s %s\n", Gray("Welcome to"), Yellow("LETAN MEDIA"))
	fmt.Printf("  %s %s\n\n", Gray("CLI Version"), Cyan(version))
}

// ─── Workflow Diagram ───
type RoleStatus struct {
	Name   string
	Model  string
	Role   string
	Status string // Ready | Waiting | Running | Done | Error
}

func PrintWorkflowDiagram(roles []RoleStatus) {
	fmt.Println(Cyan(" ═══════════════════════════════════════════════════════════════"))
	fmt.Println(Cyan("                   ORCHESTRATOR WORKFLOW"))
	fmt.Println(Cyan(" ═══════════════════════════════════════════════════════════════"))
	fmt.Println()

	// Build mini boxes inline
	boxes := make([][]string, len(roles))
	maxH := 0
	for i, r := range roles {
		boxes[i] = renderRoleBox(r)
		if len(boxes[i]) > maxH {
			maxH = len(boxes[i])
		}
	}

	// Print boxes side by side with connectors
	for row := 0; row < maxH; row++ {
		line := ""
		for i, b := range boxes {
			if row < len(b) {
				line += b[row]
			} else {
				line += strings.Repeat(" ", 22)
			}
			if i < len(boxes)-1 {
				if row == maxH/2 {
					line += Cyan(" ───► ")
				} else {
					line += "      "
				}
			}
		}
		fmt.Println(line)
	}
	fmt.Println()
}

func renderRoleBox(r RoleStatus) []string {
	statusColor := GreenLight
	if r.Status == "Waiting" {
		statusColor = YellowLight
	} else if r.Status == "Running" {
		statusColor = CyanLight
	} else if r.Status == "Error" {
		statusColor = RedLight
	} else if r.Status == "Done" {
		statusColor = GreenLight
	}

	icon := "◉"
	if r.Name == "COO" {
		icon = Cyan("◉")
	} else if r.Name == "PM" {
		icon = Yellow("◉")
	} else if r.Name == "DEV" {
		icon = Green("◉")
	} else if r.Name == "QA" {
		icon = Magenta("◉")
	} else if r.Name == "DONE" {
		icon = HiWhite("◉")
	}

	box := []string{
		fmt.Sprintf(" %s%s%s%s%s ", TL, strings.Repeat(H, 18), TM, strings.Repeat(H, 2), TR),
		fmt.Sprintf(" %s %s %-15s %s", V, icon, White(r.Name), V),
		fmt.Sprintf(" %s%s%s%s%s ", ML, strings.Repeat(H, 18), MM, strings.Repeat(H, 2), MR),
		fmt.Sprintf(" %s %-17s %s", V, Gray(r.Model), V),
		fmt.Sprintf(" %s %-17s %s", V, Gray(r.Role), V),
		fmt.Sprintf(" %s%s%s%s%s ", ML, strings.Repeat(H, 18), MM, strings.Repeat(H, 2), MR),
		fmt.Sprintf(" %s %-10s %s %s", V, "Status:", statusColor(r.Status), V),
		fmt.Sprintf(" %s%s%s%s%s ", BL, strings.Repeat(H, 18), BM, strings.Repeat(H, 2), BR),
	}
	return box
}

// ─── Live Log ───
func PrintLiveLogHeader() {
	fmt.Println(Cyan(" ┌─────────────────────────────────────────────────────────────────────────────┐"))
	fmt.Println(Cyan(" │") + " 📋 LIVE LOG" + strings.Repeat(" ", 62) + Cyan("│"))
	fmt.Println(Cyan(" ├─────────────────────────────────────────────────────────────────────────────┤"))
}

func PrintLiveLogFooter() {
	fmt.Println(Cyan(" └─────────────────────────────────────────────────────────────────────────────┘"))
}

func PrintLiveLogEntry(timestamp, actor, message string, ok bool) {
	icon := Green("✔")
	if !ok {
		icon = Red("✘")
	}
	actorColor := White
	switch actor {
	case "COO":
		actorColor = Cyan
	case "PM":
		actorColor = Yellow
	case "DEV":
		actorColor = Green
	case "QA":
		actorColor = Magenta
	}
	msg := fmt.Sprintf(" %s │ %s │ %-50s %s",
		Gray(timestamp), actorColor(fmt.Sprintf("%-4s", actor)), message, icon)
	fmt.Println(Cyan(" │") + msg + Cyan("│"))
}

// ─── Status Bar ───
func PrintStatusBar(user, path, branch string, latency time.Duration) {
	latencyStr := fmt.Sprintf("%dms", latency.Milliseconds())
	left := fmt.Sprintf(" %s@%s  %s  (%s) ", user, Yellow("toolnet"), Gray(path), Cyan(branch))
	right := fmt.Sprintf(" Latency: %s ", Green(latencyStr))
	width := 79
	mid := width - len(left) - len(right)
	if mid < 0 {
		mid = 0
	}
	bar := left + strings.Repeat(" ", mid) + right
	fmt.Println(Cyan(" ═══════════════════════════════════════════════════════════════════════════════"))
	fmt.Println(Cyan(" ║") + bar + Cyan("║"))
	fmt.Println(Cyan(" ╚═══════════════════════════════════════════════════════════════════════════════╝"))
}

// ─── Section Divider ───
func PrintSection(title string) {
	fmt.Println()
	fmt.Printf(" %s %s %s\n", Cyan("▶"), White(title), Cyan(strings.Repeat("─", 60-len(title))))
}

// ─── Prompt ───
func PrintPrompt() {
	fmt.Println()
	fmt.Printf(" %s %s %s ", Gray(">"), Cyan("Enter a command or @ to mention files..."), Gray("▌"))
}

// ─── Help Bar ───
func PrintHelpBar() {
	fmt.Println(Gray(" /help  Show commands    /setting  Open settings    ↑↓  Navigate history    Enter  Send    Esc  Exit"))
}
